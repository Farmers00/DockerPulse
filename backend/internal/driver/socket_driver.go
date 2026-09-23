package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type hostMount struct {
	Source      string
	Destination string
}

type SocketDriver struct {
	client     *DockerClient
	mountsMu   sync.Mutex
	mounts     []hostMount
	mountsInit bool
}

func NewSocketDriver(hostAddress string) (*SocketDriver, error) {
	c, err := NewDockerClient(hostAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &SocketDriver{client: c}, nil
}

func (d *SocketDriver) Ping(ctx context.Context) error {
	return d.client.Get(ctx, "/_ping", nil)
}

func (d *SocketDriver) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	var info struct {
		Name            string `json:"Name"`
		OperatingSystem string `json:"OperatingSystem"`
		KernelVersion   string `json:"KernelVersion"`
		ServerVersion   string `json:"ServerVersion"`
		NCPU            int    `json:"NCPU"`
		MemTotal        int64  `json:"MemTotal"`
	}
	if err := d.client.Get(ctx, "/info", &info); err != nil {
		return nil, err
	}

	return &SystemInfo{
		Hostname:      info.Name,
		OS:            info.OperatingSystem,
		KernelVersion: info.KernelVersion,
		DockerVersion: info.ServerVersion,
		TotalCPUs:     info.NCPU,
		TotalRAMBytes: uint64(info.MemTotal),
	}, nil
}

type rawContainer struct {
	ID      string            `json:"Id"`
	Names   []string          `json:"Names"`
	Image   string            `json:"Image"`
	ImageID string            `json:"ImageID"`
	Command string            `json:"Command"`
	Created int64             `json:"Created"`
	State   string            `json:"State"`
	Status  string            `json:"Status"`
	Labels  map[string]string `json:"Labels"`
	Ports   []struct {
		IP          string `json:"IP"`
		PrivatePort uint16 `json:"PrivatePort"`
		PublicPort  uint16 `json:"PublicPort"`
		Type        string `json:"Type"`
	} `json:"Ports"`
}

func (d *SocketDriver) ListContainers(ctx context.Context) ([]ContainerInfo, error) {
	var raw []rawContainer
	if err := d.client.Get(ctx, "/containers/json?all=1", &raw); err != nil {
		return nil, err
	}

	// Pre-fetch images to map ImageID and tags to RepoDigests
	type rawImageSummary struct {
		ID          string   `json:"Id"`
		RepoTags    []string `json:"RepoTags"`
		RepoDigests []string `json:"RepoDigests"`
	}
	var rawImages []rawImageSummary
	_ = d.client.Get(ctx, "/images/json?all=1", &rawImages)

	imageDigests := make(map[string][]string)
	imageTags := make(map[string][]string)
	tagToImageID := make(map[string]string)
	for _, img := range rawImages {
		if len(img.RepoDigests) > 0 {
			imageDigests[img.ID] = img.RepoDigests
			imageDigests[cleanDigest(img.ID)] = img.RepoDigests
			for _, tag := range img.RepoTags {
				imageDigests[tag] = img.RepoDigests
			}
		}
		if len(img.RepoTags) > 0 {
			imageTags[img.ID] = img.RepoTags
			imageTags[cleanDigest(img.ID)] = img.RepoTags
			for _, tag := range img.RepoTags {
				if tag != "" && tag != "<none>:<none>" {
					tagToImageID[tag] = img.ID
					if strings.HasSuffix(tag, ":latest") {
						tagToImageID[strings.TrimSuffix(tag, ":latest")] = img.ID
					}
					trimmed := strings.TrimPrefix(tag, "docker.io/")
					trimmed = strings.TrimPrefix(trimmed, "library/")
					tagToImageID[trimmed] = img.ID
					if strings.HasSuffix(trimmed, ":latest") {
						tagToImageID[strings.TrimSuffix(trimmed, ":latest")] = img.ID
					}
				}
			}
		}
	}

	result := make([]ContainerInfo, 0, len(raw))
	for _, c := range raw {
		// Identify internal DockerPulse updater helper containers
		isInternalHelper := c.Labels["com.dockerpulse.helper"] == "true" || strings.Contains(c.Command, "sleep 2 && docker compose")
		if !isInternalHelper {
			for _, name := range c.Names {
				trimmed := strings.TrimPrefix(name, "/")
				if strings.HasPrefix(trimmed, "dockerpulse-updater") {
					isInternalHelper = true
					break
				}
			}
		}

		if isInternalHelper {
			// If it's done or stuck in created/exited/dead state, asynchronously clean it up
			if c.State == "created" || c.State == "exited" || c.State == "dead" {
				go func(cid string) {
					_ = d.client.Delete(context.Background(), fmt.Sprintf("/containers/%s?force=true", cid))
				}(c.ID)
			}
			continue
		}

		displayImage := c.Image
		if strings.HasPrefix(displayImage, "sha256:") || displayImage == "" {
			if composeImg := c.Labels["com.docker.compose.image"]; composeImg != "" && !strings.HasPrefix(composeImg, "sha256:") {
				displayImage = composeImg
			} else if tags := imageTags[c.ImageID]; len(tags) > 0 {
				for _, t := range tags {
					if t != "" && t != "<none>:<none>" && !strings.HasPrefix(t, "sha256:") {
						displayImage = t
						break
					}
				}
			} else if tags := imageTags[cleanDigest(c.ImageID)]; len(tags) > 0 {
				for _, t := range tags {
					if t != "" && t != "<none>:<none>" && !strings.HasPrefix(t, "sha256:") {
						displayImage = t
						break
					}
				}
			}

			// If still sha256 or empty, inspect container to read Config.Image
			if strings.HasPrefix(displayImage, "sha256:") || displayImage == "" {
				var detail struct {
					Config struct {
						Image string `json:"Image"`
					} `json:"Config"`
				}
				if err := d.client.Get(ctx, "/containers/"+c.ID+"/json", &detail); err == nil {
					if detail.Config.Image != "" && !strings.HasPrefix(detail.Config.Image, "sha256:") {
						displayImage = detail.Config.Image
					}
				}
			}
		}

		// Check if local image repository tag has already been updated to a newer ID than the container's running image
		hasLocalUpdate := false
		lookupTag := strings.TrimPrefix(displayImage, "docker.io/")
		lookupTag = strings.TrimPrefix(lookupTag, "library/")
		localTagID := tagToImageID[lookupTag]
		if localTagID == "" && !strings.Contains(lookupTag, ":") {
			localTagID = tagToImageID[lookupTag+":latest"]
		}
		if localTagID != "" && cleanDigest(localTagID) != cleanDigest(c.ImageID) {
			hasLocalUpdate = true
		}

		digests := imageDigests[c.ImageID]
		if len(digests) == 0 {
			digests = imageDigests[cleanDigest(c.ImageID)]
		}
		if len(digests) == 0 {
			digests = imageDigests[c.Image]
		}

		info := ContainerInfo{
			ID:          c.ID,
			Names:       c.Names,
			Image:       displayImage,
			ImageID:     c.ImageID,
			RepoDigests: digests,
			Command:     c.Command,
			Created:     c.Created,
			State:       c.State,
			Status:      c.Status,
			HasUpdate:   hasLocalUpdate,
			Stack:       c.Labels["com.docker.compose.project"],
			Service:     c.Labels["com.docker.compose.service"],
			WorkingDir:  c.Labels["com.docker.compose.project.working_dir"],
			ConfigFile:  c.Labels["com.docker.compose.project.config_files"],
		}

		for _, p := range c.Ports {
			info.Ports = append(info.Ports, ContainerPort{
				IP:          p.IP,
				PrivatePort: p.PrivatePort,
				PublicPort:  p.PublicPort,
				Type:        p.Type,
			})
		}

		result = append(result, info)
	}

	// Fetch quick stats in parallel for running containers with a 1.5s overall cap
	var wg sync.WaitGroup
	statsCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	for i := range result {
		if result[i].State == "running" {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				d.populateContainerStats(statsCtx, result[idx].ID, &result[idx])
			}(i)
		}
	}
	wg.Wait()

	return result, nil
}

func (d *SocketDriver) populateContainerStats(ctx context.Context, id string, info *ContainerInfo) {
	statsCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var stats struct {
		CPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemUsage uint64 `json:"system_cpu_usage"`
			OnlineCPUs  uint32 `json:"online_cpus"`
		} `json:"cpu_stats"`
		PreCPUStats struct {
			CPUUsage struct {
				TotalUsage uint64 `json:"total_usage"`
			} `json:"cpu_usage"`
			SystemUsage uint64 `json:"system_cpu_usage"`
		} `json:"precpu_stats"`
		MemoryStats struct {
			Usage uint64                 `json:"usage"`
			Limit uint64                 `json:"limit"`
			Stats map[string]interface{} `json:"stats"`
		} `json:"memory_stats"`
		Networks map[string]struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		} `json:"networks"`
	}

	if err := d.client.Get(statsCtx, fmt.Sprintf("/containers/%s/stats?stream=false", id), &stats); err != nil {
		return
	}

	cpuDelta := float64(stats.CPUStats.CPUUsage.TotalUsage) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
	systemDelta := float64(stats.CPUStats.SystemUsage) - float64(stats.PreCPUStats.SystemUsage)
	onlineCPUs := float64(stats.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}

	if systemDelta > 0 && cpuDelta > 0 {
		cpuPct := (cpuDelta / systemDelta) * onlineCPUs * 100.0
		info.CPUPct = float64(int(cpuPct*100)) / 100
	}

	usedMem := stats.MemoryStats.Usage
	info.MemoryMB = float64(usedMem) / (1024 * 1024)
	if stats.MemoryStats.Limit > 0 {
		info.MemoryPct = float64(int((float64(usedMem)/float64(stats.MemoryStats.Limit))*10000)) / 100
	}

	for _, n := range stats.Networks {
		info.NetInputMB += float64(n.RxBytes) / (1024 * 1024)
		info.NetOutputMB += float64(n.TxBytes) / (1024 * 1024)
	}
}

func (d *SocketDriver) StartContainer(ctx context.Context, id string) error {
	return d.client.Post(ctx, fmt.Sprintf("/containers/%s/start", id), nil, nil)
}

func (d *SocketDriver) StopContainer(ctx context.Context, id string) error {
	return d.client.Post(ctx, fmt.Sprintf("/containers/%s/stop?t=15", id), nil, nil)
}

func (d *SocketDriver) RestartContainer(ctx context.Context, id string) error {
	return d.client.Post(ctx, fmt.Sprintf("/containers/%s/restart?t=15", id), nil, nil)
}

func (d *SocketDriver) RemoveContainer(ctx context.Context, id string, force bool) error {
	return d.client.Delete(ctx, fmt.Sprintf("/containers/%s?force=%t", id, force))
}

func (d *SocketDriver) StreamLogs(ctx context.Context, containerID string, follow bool, tail string, writer io.Writer) error {
	if tail == "" {
		tail = "200"
	}
	path := fmt.Sprintf("/containers/%s/logs?stdout=1&stderr=1&timestamps=1&follow=%t&tail=%s", containerID, follow, tail)
	return d.client.Stream(ctx, path, writer)
}

func (d *SocketDriver) ExecShell(ctx context.Context, containerID string, cmd []string, in io.Reader, out io.Writer, resizeChan <-chan TerminalSize) error {
	if len(cmd) == 0 {
		cmd = []string{"/bin/sh"}
	}

	// Local exec fallback using docker exec directly
	args := append([]string{"exec", "-i", "-t", containerID}, cmd...)
	execCmd := exec.CommandContext(ctx, "docker", args...)
	execCmd.Stdin = in
	execCmd.Stdout = out
	execCmd.Stderr = out
	return execCmd.Run()
}

func (d *SocketDriver) ListNetworks(ctx context.Context) ([]NetworkInfo, error) {
	var raw []struct {
		ID       string `json:"Id"`
		Name     string `json:"Name"`
		Driver   string `json:"Driver"`
		Scope    string `json:"Scope"`
		Internal bool   `json:"Internal"`
		IPAM     struct {
			Config []struct {
				Subnet  string `json:"Subnet"`
				Gateway string `json:"Gateway"`
			} `json:"Config"`
		} `json:"IPAM"`
		Containers map[string]struct {
			IPv4Address string `json:"IPv4Address"`
		} `json:"Containers"`
	}

	if err := d.client.Get(ctx, "/networks", &raw); err != nil {
		return nil, err
	}

	result := make([]NetworkInfo, 0, len(raw))
	for _, n := range raw {
		netInfo := NetworkInfo{
			ID:         n.ID,
			Name:       n.Name,
			Driver:     n.Driver,
			Scope:      n.Scope,
			Internal:   n.Internal,
			Containers: make(map[string]string),
		}
		if len(n.IPAM.Config) > 0 {
			netInfo.Subnet = n.IPAM.Config[0].Subnet
			netInfo.Gateway = n.IPAM.Config[0].Gateway
		}
		for cID, c := range n.Containers {
			netInfo.Containers[cID] = c.IPv4Address
		}
		result = append(result, netInfo)
	}
	return result, nil
}

func (d *SocketDriver) CreateNetwork(ctx context.Context, name, driver string) error {
	if driver == "" {
		driver = "bridge"
	}
	body := map[string]interface{}{
		"Name":   name,
		"Driver": driver,
	}
	return d.client.Post(ctx, "/networks/create", body, nil)
}

func (d *SocketDriver) RemoveNetwork(ctx context.Context, id string) error {
	return d.client.Delete(ctx, "/networks/"+id)
}

func (d *SocketDriver) ConnectNetwork(ctx context.Context, netID, containerID string) error {
	body := map[string]interface{}{
		"Container": containerID,
	}
	return d.client.Post(ctx, fmt.Sprintf("/networks/%s/connect", netID), body, nil)
}

func (d *SocketDriver) DisconnectNetwork(ctx context.Context, netID, containerID string) error {
	body := map[string]interface{}{
		"Container": containerID,
		"Force":     false,
	}
	return d.client.Post(ctx, fmt.Sprintf("/networks/%s/disconnect", netID), body, nil)
}

func (d *SocketDriver) GetDiskUsage(ctx context.Context) (*DiskUsageInfo, error) {
	var raw struct {
		LayersSize int64 `json:"LayersSize"`
		Images     []struct {
			Size      int64    `json:"Size"`
			RepoTags  []string `json:"RepoTags"`
		} `json:"Images"`
		Containers []struct {
			SizeRw int64 `json:"SizeRw"`
		} `json:"Containers"`
		Volumes []struct {
			UsageData struct {
				Size     int64 `json:"Size"`
				RefCount int   `json:"RefCount"`
			} `json:"UsageData"`
		} `json:"Volumes"`
		BuildCache []struct {
			Size int64 `json:"Size"`
		} `json:"BuildCache"`
	}

	if err := d.client.Get(ctx, "/system/df", &raw); err != nil {
		return &DiskUsageInfo{}, nil
	}

	info := &DiskUsageInfo{
		LayersSize: raw.LayersSize,
	}

	for _, img := range raw.Images {
		info.ImagesSize += img.Size
		if len(img.RepoTags) == 0 || img.RepoTags[0] == "<none>:<none>" {
			info.DanglingImages++
		}
	}

	for _, c := range raw.Containers {
		info.ContainersSize += c.SizeRw
	}

	for _, v := range raw.Volumes {
		info.VolumesSize += v.UsageData.Size
		if v.UsageData.RefCount == 0 {
			info.UnusedVolumes++
		}
	}

	for _, b := range raw.BuildCache {
		info.BuildCacheSize += b.Size
	}

	return info, nil
}

func (d *SocketDriver) PruneResources(ctx context.Context, pruneAll bool) (*PruneReport, error) {
	report := &PruneReport{}

	var imgReport struct {
		ImagesDeleted []struct {
			Untagged string `json:"Untagged"`
			Deleted  string `json:"Deleted"`
		} `json:"ImagesDeleted"`
		SpaceReclaimed int64 `json:"SpaceReclaimed"`
	}
	imgURL := "/images/prune?filters=%7B%22dangling%22%3A%5B%22true%22%5D%7D"
	if pruneAll {
		imgURL = "/images/prune"
	}
	if err := d.client.Post(ctx, imgURL, nil, &imgReport); err == nil {
		report.ImagesDeleted = len(imgReport.ImagesDeleted)
		report.SpaceReclaimed = imgReport.SpaceReclaimed
	}

	_ = d.client.Post(ctx, "/containers/prune", nil, nil)
	_ = d.client.Post(ctx, "/volumes/prune", nil, nil)

	return report, nil
}

func normalizePath(p string) string {
	if p == "" {
		return ""
	}
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	p = filepath.Clean(p)
	return strings.ReplaceAll(p, "\\", "/")
}

func (d *SocketDriver) getMounts(ctx context.Context) []hostMount {
	d.mountsMu.Lock()
	defer d.mountsMu.Unlock()

	if d.mountsInit {
		return d.mounts
	}
	d.mountsInit = true

	// 1. Check environment variable override
	if envHostBase := os.Getenv("HOST_BASE_DIR"); envHostBase != "" {
		envContainerBase := os.Getenv("CONTAINER_BASE_DIR")
		if envContainerBase == "" {
			envContainerBase = "/root/docker"
		}
		d.mounts = append(d.mounts, hostMount{
			Source:      normalizePath(envHostBase),
			Destination: normalizePath(envContainerBase),
		})
	}

	// 2. Query Docker inspect for self-container
	hostname, _ := os.Hostname()
	candidates := []string{
		hostname,
		os.Getenv("HOSTNAME"),
		"dockerpulse-agent",
		"dockerpulse",
	}

	type inspectResp struct {
		Mounts []struct {
			Type        string `json:"Type"`
			Source      string `json:"Source"`
			Destination string `json:"Destination"`
		} `json:"Mounts"`
	}

	for _, cid := range candidates {
		if cid == "" {
			continue
		}
		var resp inspectResp
		if err := d.client.Get(ctx, "/containers/"+cid+"/json", &resp); err == nil && len(resp.Mounts) > 0 {
			for _, m := range resp.Mounts {
				if m.Type == "bind" && !strings.Contains(m.Source, "docker.sock") && !strings.Contains(m.Destination, "docker.sock") {
					normSrc := normalizePath(m.Source)
					normDst := normalizePath(m.Destination)
					exists := false
					for _, existing := range d.mounts {
						if existing.Destination == normDst {
							exists = true
							break
						}
					}
					if !exists {
						d.mounts = append(d.mounts, hostMount{
							Source:      normSrc,
							Destination: normDst,
						})
					}
				}
			}
			if len(d.mounts) > 0 {
				break
			}
		}
	}

	// 3. Fallback: check /containers/json for dockerpulse containers
	if len(d.mounts) == 0 {
		var containers []struct {
			Names  []string `json:"Names"`
			Image  string   `json:"Image"`
			Mounts []struct {
				Type        string `json:"Type"`
				Source      string `json:"Source"`
				Destination string `json:"Destination"`
			} `json:"Mounts"`
		}
		if err := d.client.Get(ctx, "/containers/json?all=1", &containers); err == nil {
			for _, c := range containers {
				isSelf := strings.Contains(strings.ToLower(c.Image), "dockerpulse")
				if !isSelf {
					for _, n := range c.Names {
						if strings.Contains(strings.ToLower(n), "dockerpulse") {
							isSelf = true
							break
						}
					}
				}
				if isSelf && len(c.Mounts) > 0 {
					for _, m := range c.Mounts {
						if m.Type == "bind" && !strings.Contains(m.Source, "docker.sock") && !strings.Contains(m.Destination, "docker.sock") {
							normSrc := normalizePath(m.Source)
							normDst := normalizePath(m.Destination)
							exists := false
							for _, existing := range d.mounts {
								if existing.Destination == normDst {
									exists = true
									break
								}
							}
							if !exists {
								d.mounts = append(d.mounts, hostMount{
									Source:      normSrc,
									Destination: normDst,
								})
							}
						}
					}
					if len(d.mounts) > 0 {
						break
					}
				}
			}
		}
	}

	return d.mounts
}

func (d *SocketDriver) ToHostPath(ctx context.Context, p string) string {
	if p == "" {
		return ""
	}
	normP := normalizePath(p)
	mounts := d.getMounts(ctx)

	// Check if normP already matches a host mount Source
	for _, m := range mounts {
		if normP == m.Source || strings.HasPrefix(normP, m.Source+"/") {
			return normP
		}
	}

	// Check if normP matches a container mount Destination
	for _, m := range mounts {
		if normP == m.Destination {
			return m.Source
		}
		if strings.HasPrefix(normP, m.Destination+"/") {
			rel := strings.TrimPrefix(normP, m.Destination+"/")
			return m.Source + "/" + rel
		}
	}

	return normP
}

func (d *SocketDriver) ToContainerPath(ctx context.Context, p string) string {
	if p == "" {
		return ""
	}
	expanded := expandHomeDir(p)
	normP := normalizePath(expanded)
	mounts := d.getMounts(ctx)

	// If normP already matches a container mount Destination, return it
	for _, m := range mounts {
		if normP == m.Destination || strings.HasPrefix(normP, m.Destination+"/") {
			return normP
		}
	}

	// If normP matches a host mount Source, convert to container Destination
	for _, m := range mounts {
		if normP == m.Source {
			return m.Destination
		}
		if strings.HasPrefix(normP, m.Source+"/") {
			rel := strings.TrimPrefix(normP, m.Source+"/")
			return m.Destination + "/" + rel
		}
	}

	return normP
}

// Stacks filesystem discovery and compose execution
func (d *SocketDriver) DiscoverStacks(ctx context.Context, baseDir string) ([]DiscoveredStack, error) {
	containerBase := d.ToContainerPath(ctx, baseDir)
	entries, err := os.ReadDir(containerBase)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []DiscoveredStack{}, nil
		}
		return nil, err
	}

	var stacks []DiscoveredStack
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		containerDirPath := filepath.Join(containerBase, entry.Name())
		composeFile := findComposeFile(containerDirPath)
		if composeFile != "" {
			info, _ := entry.Info()
			hasEnv := false
			if _, err := os.Stat(filepath.Join(containerDirPath, ".env")); err == nil {
				hasEnv = true
			}

			hostDirPath := d.ToHostPath(ctx, containerDirPath)

			stacks = append(stacks, DiscoveredStack{
				Name:        entry.Name(),
				Path:        hostDirPath,
				ComposeFile: filepath.Base(composeFile),
				HasEnvFile:  hasEnv,
				UpdatedAt:   info.ModTime(),
			})
		}
	}
	return stacks, nil
}

func (d *SocketDriver) ReadStackFiles(ctx context.Context, stackPath string) (string, string, error) {
	containerPath := d.ToContainerPath(ctx, stackPath)
	composeFile := findComposeFile(containerPath)
	if composeFile == "" {
		return "", "", fmt.Errorf("no compose file found in %s", stackPath)
	}

	composeBytes, err := os.ReadFile(composeFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to read compose file: %w", err)
	}

	envBytes, _ := os.ReadFile(filepath.Join(containerPath, ".env"))
	return string(composeBytes), string(envBytes), nil
}

func (d *SocketDriver) WriteStackFiles(ctx context.Context, stackPath string, composeContent string, envContent string) error {
	containerPath := d.ToContainerPath(ctx, stackPath)
	composeFile := findComposeFile(containerPath)
	if composeFile == "" {
		composeFile = filepath.Join(containerPath, "docker-compose.yml")
	}

	if err := os.MkdirAll(containerPath, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(composeFile, []byte(composeContent), 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	if envContent != "" || fileExists(filepath.Join(containerPath, ".env")) {
		if err := os.WriteFile(filepath.Join(containerPath, ".env"), []byte(envContent), 0644); err != nil {
			return fmt.Errorf("failed to write .env file: %w", err)
		}
	}

	return nil
}

func isSelfUpdate(containerPath string) bool {
	composeFile := findComposeFile(containerPath)
	if composeFile != "" {
		content, err := os.ReadFile(composeFile)
		if err == nil && strings.Contains(strings.ToLower(string(content)), "dockerpulse") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(containerPath), "dockerpulse")
}

func (d *SocketDriver) ExecuteCompose(ctx context.Context, stackPath string, action string, writer io.Writer) error {
	containerPath := d.ToContainerPath(ctx, stackPath)
	hostPath := d.ToHostPath(ctx, stackPath)
	composeFile := findComposeFile(containerPath)
	if composeFile == "" {
		return fmt.Errorf("no compose file found in %s", stackPath)
	}

	projectName := sanitizeComposeName(filepath.Base(hostPath))
	envFile := filepath.Join(containerPath, ".env")

	// Detect top-level 'name:' issues and notify user, without automatically modifying user files
	if hasIssue, oldName, proposedName := detectComposeNameIssue(composeFile); hasIssue {
		fmt.Fprintf(writer, "[DockPulse] WARNING: Top-level 'name: %s' in %s violates Docker Compose v2 naming rules (pattern '^[a-z0-9][a-z0-9_-]*$').\n", oldName, filepath.Base(composeFile))
		fmt.Fprintf(writer, "[DockPulse] Suggested fix: 'name: %s'. You can apply this fix or edit the file in DockerPulse.\n", proposedName)
	}

	switch action {
	case "up", "restart":
		if isSelfUpdate(containerPath) {
			fmt.Fprintf(writer, "[DockPulse] Detected self-update/restart of DockerPulse at %s\n", hostPath)
			fmt.Fprintln(writer, "[DockPulse] Spawning detached helper runner to safely restart container...")

			selfRef, _ := os.Hostname()
			if selfRef == "" {
				selfRef = "dockerpulse-agent"
			}

			composeSubCmd := "up -d"
			if action == "restart" {
				composeSubCmd = "restart"
			}

			cmdScript := fmt.Sprintf("sleep 2 && docker compose -p %s -f %s --project-directory %s %s",
				projectName,
				composeFile,
				hostPath,
				composeSubCmd,
			)
			// Ensure any previous helper container with this name is removed first
			_ = exec.Command("docker", "rm", "-f", "dockerpulse-updater-helper").Run()

			runnerCmd := exec.Command("docker", "run", "--rm", "-d",
				"--name", "dockerpulse-updater-helper",
				"--label", "com.dockerpulse.helper=true",
				"--entrypoint", "sh",
				"-v", "/var/run/docker.sock:/var/run/docker.sock",
				"--volumes-from", selfRef,
				"-w", containerPath,
				"ghcr.io/farmers00/dockerpulse:latest",
				"-c", cmdScript,
			)

			out, err := runnerCmd.CombinedOutput()
			if err != nil {
				_ = exec.Command("docker", "rm", "-f", "dockerpulse-updater-helper").Run()
				if selfRef != "dockerpulse-agent" {
					runnerCmd2 := exec.Command("docker", "run", "--rm", "-d",
						"--name", "dockerpulse-updater-helper",
						"--label", "com.dockerpulse.helper=true",
						"--entrypoint", "sh",
						"-v", "/var/run/docker.sock:/var/run/docker.sock",
						"--volumes-from", "dockerpulse-agent",
						"-w", containerPath,
						"ghcr.io/farmers00/dockerpulse:latest",
						"-c", cmdScript,
					)
					if out2, err2 := runnerCmd2.CombinedOutput(); err2 == nil {
						out = out2
						err = nil
					} else {
						_ = exec.Command("docker", "rm", "-f", "dockerpulse-updater-helper").Run()
					}
				}
			}

			if err == nil {
				cid := strings.TrimSpace(string(out))
				if len(cid) > 12 {
					cid = cid[:12]
				}
				fmt.Fprintf(writer, "[DockPulse] Detached runner container launched (ID: %s).\n", cid)
				fmt.Fprintf(writer, "[DockPulse] Container will %s in 2 seconds with updated image.\n", action)
				fmt.Fprintln(writer, "[DockPulse] Command completed successfully.")
				return nil
			}

			fmt.Fprintf(writer, "[DockPulse] Detached runner failed (%v: %s), falling back to direct compose\n", err, strings.TrimSpace(string(out)))
		}

		args := []string{"compose", "-p", projectName, "-f", composeFile}
		if fileExists(envFile) {
			args = append(args, "--env-file", envFile)
		}
		if hostPath != "" {
			args = append(args, "--project-directory", hostPath)
		}
		args = append(args, action)
		if action == "up" {
			args = append(args, "-d")
		}

		return d.runComposeCmd(ctx, containerPath, args, writer)

	case "down":
		args := []string{"compose", "-p", projectName, "-f", composeFile}
		if fileExists(envFile) {
			args = append(args, "--env-file", envFile)
		}
		if hostPath != "" {
			args = append(args, "--project-directory", hostPath)
		}
		args = append(args, "down")
		return d.runComposeCmd(ctx, containerPath, args, writer)

	case "pull":
		args := []string{"compose", "-p", projectName, "-f", composeFile}
		if fileExists(envFile) {
			args = append(args, "--env-file", envFile)
		}
		if hostPath != "" {
			args = append(args, "--project-directory", hostPath)
		}
		args = append(args, "pull")
		return d.runComposeCmd(ctx, containerPath, args, writer)

	case "pull_up":
		if err := d.ExecuteCompose(ctx, stackPath, "pull", writer); err != nil {
			return err
		}
		return d.ExecuteCompose(ctx, stackPath, "up", writer)

	default:
		return fmt.Errorf("unsupported compose action: %s", action)
	}
}

func (d *SocketDriver) runComposeCmd(ctx context.Context, workingDir string, args []string, writer io.Writer) error {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = workingDir
	cmd.Stdout = writer
	cmd.Stderr = writer

	fmt.Fprintf(writer, "[DockPulse] Running: docker %s\n", strings.Join(args, " "))
	err := cmd.Run()
	if err != nil {
		fmt.Fprintf(writer, "[DockPulse] Command finished with error: %v\n", err)
	} else {
		fmt.Fprintln(writer, "[DockPulse] Command completed successfully.")
	}
	return err
}

func (d *SocketDriver) Close() error {
	return nil
}

func findComposeFile(dir string) string {
	candidates := []string{
		"docker-compose.yml",
		"docker-compose.yaml",
		"compose.yml",
		"compose.yaml",
	}
	for _, c := range candidates {
		p := filepath.Join(dir, c)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

func expandHomeDir(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, path[1:])
		}
	}
	return path
}

func cleanDigest(d string) string {
	d = strings.TrimSpace(d)
	d = strings.Trim(d, "\"")
	if idx := strings.LastIndex(d, "sha256:"); idx != -1 {
		return d[idx+7:]
	}
	return strings.TrimPrefix(d, "sha256:")
}

var (
	validComposeNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)
	topLevelNameRegex     = regexp.MustCompile(`(?m)^name\s*:\s*(.+)$`)
)

func sanitizeComposeName(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, `"'`)
	raw = strings.ToLower(raw)

	var b strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == '-' || r == '_' || r == ' ' || r == '.' {
			if b.Len() > 0 {
				last := b.String()[b.Len()-1]
				if last != '-' && last != '_' {
					b.WriteByte('-')
				}
			}
		}
	}
	res := strings.Trim(b.String(), "-_")
	if res == "" || !((res[0] >= 'a' && res[0] <= 'z') || (res[0] >= '0' && res[0] <= '9')) {
		return "stack"
	}
	return res
}

func sanitizeComposeContent(content string) (newContent string, modified bool, oldName string, newName string) {
	loc := topLevelNameRegex.FindStringSubmatchIndex(content)
	if len(loc) < 4 {
		return content, false, "", ""
	}

	valWithComment := content[loc[2]:loc[3]]
	val := strings.TrimSpace(strings.Split(valWithComment, "#")[0])
	val = strings.Trim(val, `"'`)

	if validComposeNameRegex.MatchString(val) {
		return content, false, "", ""
	}

	sanitized := sanitizeComposeName(val)
	fullMatch := content[loc[0]:loc[1]]
	lineEnding := ""
	if strings.HasSuffix(fullMatch, "\r") {
		lineEnding = "\r"
	}
	comment := ""
	if hashIdx := strings.Index(valWithComment, "#"); hashIdx != -1 {
		comment = " " + strings.TrimSpace(valWithComment[hashIdx:])
	}
	newLine := "name: " + sanitized + comment + lineEnding
	newContent = content[:loc[0]] + newLine + content[loc[1]:]
	return newContent, true, val, sanitized
}

func detectComposeNameIssue(filePath string) (hasIssue bool, oldName string, proposedName string) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return false, "", ""
	}
	_, hasIssue, oldName, proposedName = sanitizeComposeContent(string(data))
	return hasIssue, oldName, proposedName
}

