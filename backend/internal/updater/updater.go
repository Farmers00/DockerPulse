package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dockpulse/dockmgr/internal/database"
	"github.com/dockpulse/dockmgr/internal/driver"
)

type UpdateChecker struct {
	db     *database.DB
	client *http.Client
}

func NewUpdateChecker(db *database.DB) *UpdateChecker {
	return &UpdateChecker{
		db: db,
		client: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

type CheckResult struct {
	Image         string `json:"image"`
	CurrentDigest string `json:"current_digest"`
	RemoteDigest  string `json:"remote_digest"`
	HasUpdate     bool   `json:"has_update"`
}

type remoteManifestInfo struct {
	IndexDigest     string
	PlatformDigests []string
	ConfigDigest    string
}

// CheckImage checks whether a remote image has a newer digest than the local one
func (u *UpdateChecker) CheckImage(ctx context.Context, imageName string, currentImageID string, repoDigests []string) (*CheckResult, error) {
	// Skip raw sha256 images without a repository tag
	if strings.HasPrefix(imageName, "sha256:") {
		return &CheckResult{
			Image:         imageName,
			CurrentDigest: currentImageID,
			HasUpdate:     false,
		}, nil
	}

	// Parse image repository and tag
	registry, repo, tag := parseImageRef(imageName)

	manifestInfo, err := u.fetchRemoteManifest(ctx, registry, repo, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote digest for %s: %w", imageName, err)
	}

	hasUpdate := false
	primaryRemoteDigest := manifestInfo.IndexDigest
	if primaryRemoteDigest == "" && len(manifestInfo.PlatformDigests) > 0 {
		primaryRemoteDigest = manifestInfo.PlatformDigests[0]
	}

	if primaryRemoteDigest != "" || manifestInfo.ConfigDigest != "" {
		matched := false

		isMatch := func(a, b string) bool {
			cleanA := strings.TrimPrefix(strings.TrimSpace(a), "sha256:")
			cleanB := strings.TrimPrefix(strings.TrimSpace(b), "sha256:")
			return cleanA != "" && cleanB != "" && (cleanA == cleanB || strings.HasPrefix(cleanA, cleanB) || strings.HasPrefix(cleanB, cleanA))
		}

		// 1. Check if local currentImageID matches remote ConfigDigest
		if manifestInfo.ConfigDigest != "" && isMatch(currentImageID, manifestInfo.ConfigDigest) {
			matched = true
		}

		// 2. Check if any local repoDigests match remote IndexDigest
		if !matched && manifestInfo.IndexDigest != "" {
			cleanIdx := strings.TrimPrefix(manifestInfo.IndexDigest, "sha256:")
			for _, rd := range repoDigests {
				if strings.Contains(rd, cleanIdx) {
					matched = true
					break
				}
			}
		}

		// 3. Check if any local repoDigests match any remote PlatformDigests
		if !matched {
			for _, pd := range manifestInfo.PlatformDigests {
				cleanPd := strings.TrimPrefix(pd, "sha256:")
				for _, rd := range repoDigests {
					if strings.Contains(rd, cleanPd) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}

		// 4. Fallback: currentImageID direct comparison
		if !matched && manifestInfo.IndexDigest != "" && isMatch(currentImageID, manifestInfo.IndexDigest) {
			matched = true
		}

		// If no matches found, remote image has been updated!
		if !matched {
			hasUpdate = true
		}
	}

	return &CheckResult{
		Image:         imageName,
		CurrentDigest: currentImageID,
		RemoteDigest:  primaryRemoteDigest,
		HasUpdate:     hasUpdate,
	}, nil
}

func parseImageRef(image string) (registry, repo, tag string) {
	tag = "latest"
	if idx := strings.LastIndex(image, ":"); idx != -1 && !strings.Contains(image[idx:], "/") {
		tag = image[idx+1:]
		image = image[:idx]
	}

	parts := strings.Split(image, "/")
	switch len(parts) {
	case 1:
		registry = "registry-1.docker.io"
		repo = "library/" + parts[0]
	case 2:
		if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			registry = parts[0]
			repo = parts[1]
		} else {
			registry = "registry-1.docker.io"
			repo = parts[0] + "/" + parts[1]
		}
	default:
		registry = parts[0]
		repo = strings.Join(parts[1:], "/")
	}

	return registry, repo, tag
}

func (u *UpdateChecker) fetchRemoteManifest(ctx context.Context, registry, repo, tag string) (*remoteManifestInfo, error) {
	// 1. Get auth token if Docker Hub or GHCR
	var token string
	if registry == "registry-1.docker.io" {
		authURL := fmt.Sprintf("https://auth.docker.io/token?service=registry.docker.io&scope=repository:%s:pull", repo)
		req, _ := http.NewRequestWithContext(ctx, "GET", authURL, nil)
		resp, err := u.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var authResp struct {
				Token string `json:"token"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&authResp)
			token = authResp.Token
			resp.Body.Close()
		}
	} else if registry == "ghcr.io" {
		authURL := fmt.Sprintf("https://ghcr.io/token?service=ghcr.io&scope=repository:%s:pull", repo)
		req, _ := http.NewRequestWithContext(ctx, "GET", authURL, nil)
		resp, err := u.client.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			var authResp struct {
				Token string `json:"token"`
			}
			_ = json.NewDecoder(resp.Body).Decode(&authResp)
			token = authResp.Token
			resp.Body.Close()
		}
	}

	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, err
	}

	// Accept both manifest lists / OCI image indexes and single-arch manifests
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, err
	}

	info := &remoteManifestInfo{}
	info.IndexDigest = resp.Header.Get("Docker-Content-Digest")
	if info.IndexDigest == "" {
		etag := resp.Header.Get("ETag")
		if strings.HasPrefix(etag, "\"") && strings.HasSuffix(etag, "\"") {
			info.IndexDigest = strings.Trim(etag, "\"")
		}
	}

	var doc struct {
		MediaType string `json:"mediaType"`
		Config    struct {
			Digest string `json:"digest"`
		} `json:"config"`
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				Architecture string `json:"architecture"`
				OS           string `json:"os"`
			} `json:"platform"`
		} `json:"manifests"`
	}

	if err := json.Unmarshal(body, &doc); err == nil {
		if doc.Config.Digest != "" {
			info.ConfigDigest = doc.Config.Digest
		}
		for _, m := range doc.Manifests {
			if m.Platform.OS == "linux" {
				info.PlatformDigests = append(info.PlatformDigests, m.Digest)
			}
		}
	}

	return info, nil
}

// CheckHostContainers checks all containers on a given host
func (u *UpdateChecker) CheckHostContainers(ctx context.Context, h driver.HostDriver, hostID string) ([]CheckResult, error) {
	containers, err := h.ListContainers(ctx)
	if err != nil {
		return nil, err
	}

	var results []CheckResult
	for _, c := range containers {
		if c.Image == "" {
			continue
		}
		res, err := u.CheckImage(ctx, c.Image, c.ImageID, c.RepoDigests)
		if err == nil {
			results = append(results, *res)
		}
	}
	return results, nil
}
