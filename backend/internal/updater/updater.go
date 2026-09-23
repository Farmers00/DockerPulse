package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
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

	cleanCurrent := cleanDigest(currentImageID)
	cleanIndex := cleanDigest(manifestInfo.IndexDigest)
	cleanConfig := cleanDigest(manifestInfo.ConfigDigest)

	if cleanIndex != "" || len(manifestInfo.PlatformDigests) > 0 || cleanConfig != "" {
		matched := false

		// 1. Check if local currentImageID matches remote ConfigDigest
		if cleanConfig != "" && cleanCurrent != "" && cleanCurrent == cleanConfig {
			matched = true
		}

		// 2. Check if any local repoDigests match remote IndexDigest
		if !matched && cleanIndex != "" {
			for _, rd := range repoDigests {
				if strings.Contains(cleanDigest(rd), cleanIndex) {
					matched = true
					break
				}
			}
		}

		// 3. Check if any local repoDigests match any remote PlatformDigests
		if !matched {
			for _, pd := range manifestInfo.PlatformDigests {
				cleanPd := cleanDigest(pd)
				if cleanPd == "" {
					continue
				}
				if cleanCurrent != "" && cleanCurrent == cleanPd {
					matched = true
					break
				}
				for _, rd := range repoDigests {
					if strings.Contains(cleanDigest(rd), cleanPd) {
						matched = true
						break
					}
				}
				if matched {
					break
				}
			}
		}

		// If no remote digest matched our local image, an update is available!
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

func cleanDigest(d string) string {
	d = strings.TrimSpace(d)
	d = strings.Trim(d, "\"")
	if idx := strings.LastIndex(d, "sha256:"); idx != -1 {
		return d[idx+7:]
	}
	return strings.TrimPrefix(d, "sha256:")
}

func parseImageRef(image string) (registry, repo, tag string) {
	// Strip digest pinning if present: e.g. "repo/image:tag@sha256:..." -> "repo/image:tag"
	if atIdx := strings.Index(image, "@"); atIdx != -1 {
		image = image[:atIdx]
	}

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
		if parts[0] == "docker.io" || parts[0] == "index.docker.io" {
			registry = "registry-1.docker.io"
			repo = "library/" + parts[1]
		} else if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
			registry = parts[0]
			repo = parts[1]
		} else {
			registry = "registry-1.docker.io"
			repo = parts[0] + "/" + parts[1]
		}
	default:
		if parts[0] == "docker.io" || parts[0] == "index.docker.io" {
			registry = "registry-1.docker.io"
			repo = strings.Join(parts[1:], "/")
			if !strings.Contains(repo, "/") {
				repo = "library/" + repo
			}
		} else {
			registry = parts[0]
			repo = strings.Join(parts[1:], "/")
		}
	}

	repo = strings.ToLower(repo)
	return registry, repo, tag
}

func parseWwwAuthHeader(header string) (realm, service, scope string) {
	header = strings.TrimSpace(header)
	if !strings.HasPrefix(strings.ToLower(header), "bearer ") {
		return "", "", ""
	}
	header = header[7:]

	parts := strings.Split(header, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		kv := strings.SplitN(p, "=", 2)
		if len(kv) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(kv[0]))
		val := strings.Trim(strings.TrimSpace(kv[1]), "\"")
		switch key {
		case "realm":
			realm = val
		case "service":
			service = val
		case "scope":
			scope = val
		}
	}
	return realm, service, scope
}

func (u *UpdateChecker) fetchRemoteManifest(ctx context.Context, registry, repo, tag string) (*remoteManifestInfo, error) {
	manifestURL := fmt.Sprintf("https://%s/v2/%s/manifests/%s", registry, repo, tag)
	req, err := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}

	// If 401 Unauthorized, perform standard Registry V2 Bearer Token authentication
	if resp.StatusCode == http.StatusUnauthorized {
		resp.Body.Close()

		authHeader := resp.Header.Get("Www-Authenticate")
		realm, service, scope := parseWwwAuthHeader(authHeader)

		if realm == "" {
			// Fallback defaults for standard Docker Hub and GHCR
			if registry == "registry-1.docker.io" {
				realm = "https://auth.docker.io/token"
				service = "registry.docker.io"
				scope = fmt.Sprintf("repository:%s:pull", repo)
			} else if registry == "ghcr.io" {
				realm = "https://ghcr.io/token"
				service = "ghcr.io"
				scope = fmt.Sprintf("repository:%s:pull", repo)
			}
		}

		if realm != "" {
			tokenURL := fmt.Sprintf("%s?service=%s&scope=%s", realm, url.QueryEscape(service), url.QueryEscape(scope))
			if uRealm, parseErr := url.Parse(realm); parseErr == nil {
				q := uRealm.Query()
				if service != "" {
					q.Set("service", service)
				}
				if scope != "" {
					q.Set("scope", scope)
				}
				uRealm.RawQuery = q.Encode()
				tokenURL = uRealm.String()
			}

			tokenReq, _ := http.NewRequestWithContext(ctx, "GET", tokenURL, nil)
			tokenResp, err := u.client.Do(tokenReq)
			if err == nil && tokenResp.StatusCode == http.StatusOK {
				var authResp struct {
					Token       string `json:"token"`
					AccessToken string `json:"access_token"`
				}
				_ = json.NewDecoder(tokenResp.Body).Decode(&authResp)
				tokenResp.Body.Close()

				tok := authResp.Token
				if tok == "" {
					tok = authResp.AccessToken
				}

				if tok != "" {
					req2, _ := http.NewRequestWithContext(ctx, "GET", manifestURL, nil)
					req2.Header.Set("Accept", "application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.oci.image.manifest.v1+json")
					req2.Header.Set("Authorization", "Bearer "+tok)
					resp, err = u.client.Do(req2)
					if err != nil {
						return nil, err
					}
				}
			}
		}
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry %s returned status %d for %s:%s", registry, resp.StatusCode, repo, tag)
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

	type checkTask struct {
		container driver.ContainerInfo
		cacheKey  string
	}

	var tasks []checkTask
	seen := make(map[string]bool)

	for _, c := range containers {
		if c.Image == "" {
			continue
		}
		cacheKey := c.Image + "::" + c.ImageID
		if !seen[cacheKey] {
			seen[cacheKey] = true
			tasks = append(tasks, checkTask{container: c, cacheKey: cacheKey})
		}
	}

	var resultsMu sync.Mutex
	resMap := make(map[string]*CheckResult)

	// Concurrency limiter (up to 6 concurrent registry queries)
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup

	for _, task := range tasks {
		c := task.container
		key := task.cacheKey

		// If already known to have an update from local tag mismatch, record immediately
		if c.HasUpdate {
			resultsMu.Lock()
			resMap[key] = &CheckResult{
				Image:         c.Image,
				CurrentDigest: c.ImageID,
				HasUpdate:     true,
			}
			resultsMu.Unlock()
			continue
		}

		wg.Add(1)
		go func(c driver.ContainerInfo, key string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			res, err := u.CheckImage(ctx, c.Image, c.ImageID, c.RepoDigests)
			resultsMu.Lock()
			defer resultsMu.Unlock()

			if err != nil {
				log.Printf("[Updater] CheckImage error for host %s, image %s: %v", hostID, c.Image, err)
				if c.HasUpdate {
					resMap[key] = &CheckResult{
						Image:         c.Image,
						CurrentDigest: c.ImageID,
						HasUpdate:     true,
					}
				}
				return
			}

			if c.HasUpdate {
				res.HasUpdate = true
			}
			resMap[key] = res
		}(c, key)
	}

	wg.Wait()

	var results []CheckResult
	for _, c := range containers {
		if c.Image == "" {
			continue
		}
		key := c.Image + "::" + c.ImageID
		if res, ok := resMap[key]; ok && res != nil {
			results = append(results, *res)
		} else if c.HasUpdate {
			results = append(results, CheckResult{
				Image:         c.Image,
				CurrentDigest: c.ImageID,
				HasUpdate:     true,
			})
		}
	}

	return results, nil
}

