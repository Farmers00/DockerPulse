package updater

import (
	"context"
	"encoding/json"
	"fmt"
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

// CheckImage checks whether a remote image has a newer digest than the local one
func (u *UpdateChecker) CheckImage(ctx context.Context, imageName string, currentImageID string) (*CheckResult, error) {
	// Parse image repository and tag
	registry, repo, tag := parseImageRef(imageName)

	remoteDigest, err := u.fetchRemoteDigest(ctx, registry, repo, tag)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch remote digest for %s: %w", imageName, err)
	}

	hasUpdate := false
	if currentImageID != "" && remoteDigest != "" {
		// Image IDs often look like sha256:abcd...
		cleanCurrent := strings.TrimPrefix(currentImageID, "sha256:")
		cleanRemote := strings.TrimPrefix(remoteDigest, "sha256:")
		if !strings.HasPrefix(cleanRemote, cleanCurrent) && !strings.HasPrefix(cleanCurrent, cleanRemote) {
			hasUpdate = true
		}
	}

	return &CheckResult{
		Image:         imageName,
		CurrentDigest: currentImageID,
		RemoteDigest:  remoteDigest,
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

func (u *UpdateChecker) fetchRemoteDigest(ctx context.Context, registry, repo, tag string) (string, error) {
	// 1. Get auth token if Docker Hub
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
	req, err := http.NewRequestWithContext(ctx, "HEAD", manifestURL, nil)
	if err != nil {
		return "", err
	}

	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.list.v2+json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := u.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	return digest, nil
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
		res, err := u.CheckImage(ctx, c.Image, c.ImageID)
		if err == nil {
			results = append(results, *res)
		}
	}
	return results, nil
}
