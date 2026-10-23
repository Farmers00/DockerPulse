package driver

import (
	"context"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"time"
)

type hostMount struct {
	Source      string
	Destination string
}

type cpuSample struct {
	TotalUsage  uint64
	SystemUsage uint64
	Timestamp   time.Time
}

type containerStatsSnapshot struct {
	CPUPct      float64
	MemoryMB    float64
	MemoryPct   float64
	NetInputMB  float64
	NetOutputMB float64
	UpdatedAt   time.Time
}

type SocketDriver struct {
	client     *DockerClient
	mountsMu   sync.Mutex
	mounts     []hostMount
	mountsInit bool

	cpuMu     sync.Mutex
	prevCPUs  map[string]cpuSample
	lastStats map[string]containerStatsSnapshot
}

func NewSocketDriver(hostAddress string) (*SocketDriver, error) {
	c, err := NewDockerClient(hostAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	return &SocketDriver{
		client:    c,
		prevCPUs:  make(map[string]cpuSample),
		lastStats: make(map[string]containerStatsSnapshot),
	}, nil
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

	// Fetch quick stats in parallel for running containers with worker concurrency pool
	var wg sync.WaitGroup
	statsCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	sem := make(chan struct{}, 8)

	for i := range result {
		if result[i].State == "running" {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				select {
				case sem <- struct{}{}:
					defer func() { <-sem }()
				case <-statsCtx.Done():
					d.cpuMu.Lock()
					if cached, ok := d.lastStats[result[idx].ID]; ok {
						result[idx].CPUPct = cached.CPUPct
						result[idx].MemoryMB = cached.MemoryMB
						result[idx].MemoryPct = cached.MemoryPct
						result[idx].NetInputMB = cached.NetInputMB
						result[idx].NetOutputMB = cached.NetOutputMB
					}
					d.cpuMu.Unlock()
					return
				}
				d.populateContainerStats(statsCtx, result[idx].ID, &result[idx])
			}(i)
		}
	}
	wg.Wait()

	return result, nil
}

func parseUint64(v interface{}) uint64 {
	switch val := v.(type) {
	case float64:
		return uint64(val)
	case int64:
		return uint64(val)
	case uint64:
		return val
	case int:
		return uint64(val)
	}
	return 0
}

func (d *SocketDriver) populateContainerStats(ctx context.Context, id string, info *ContainerInfo) {
	statsCtx, cancel := context.WithTimeout(ctx, 3500*time.Millisecond)
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
		Network struct {
			RxBytes uint64 `json:"rx_bytes"`
			TxBytes uint64 `json:"tx_bytes"`
		} `json:"network"`
	}

	// First attempt with one-shot=true for instantaneous return without 1-second CPU delta blocking
	err := d.client.Get(statsCtx, fmt.Sprintf("/containers/%s/stats?stream=false&one-shot=true", id), &stats)
	if err != nil {
		// Fallback to standard stream=false if one-shot is rejected or fails
		err = d.client.Get(statsCtx, fmt.Sprintf("/containers/%s/stats?stream=false", id), &stats)
	}

	if err != nil {
		// If transient failure or timeout, fallback to last known good stats
		d.cpuMu.Lock()
		if cached, ok := d.lastStats[id]; ok {
			info.CPUPct = cached.CPUPct
			info.MemoryMB = cached.MemoryMB
			info.MemoryPct = cached.MemoryPct
			info.NetInputMB = cached.NetInputMB
			info.NetOutputMB = cached.NetOutputMB
		}
		d.cpuMu.Unlock()
		return
	}

	currentTotal := stats.CPUStats.CPUUsage.TotalUsage
	currentSystem := stats.CPUStats.SystemUsage
	onlineCPUs := float64(stats.CPUStats.OnlineCPUs)
	if onlineCPUs == 0 {
		onlineCPUs = 1
	}

	d.cpuMu.Lock()
	if d.prevCPUs == nil {
		d.prevCPUs = make(map[string]cpuSample)
	}
	prev, hasPrev := d.prevCPUs[id]
	d.prevCPUs[id] = cpuSample{
		TotalUsage:  currentTotal,
		SystemUsage: currentSystem,
		Timestamp:   time.Now(),
	}
	d.cpuMu.Unlock()

	var cpuDelta float64
	var systemDelta float64

	if hasPrev && prev.TotalUsage > 0 && currentTotal >= prev.TotalUsage {
		cpuDelta = float64(currentTotal - prev.TotalUsage)
		if currentSystem > prev.SystemUsage {
			systemDelta = float64(currentSystem - prev.SystemUsage)
		} else {
			elapsedNano := time.Since(prev.Timestamp).Nanoseconds()
			if elapsedNano > 0 {
				systemDelta = float64(elapsedNano)
			}
		}
	} else if stats.PreCPUStats.CPUUsage.TotalUsage > 0 && stats.PreCPUStats.SystemUsage > 0 {
		cpuDelta = float64(currentTotal) - float64(stats.PreCPUStats.CPUUsage.TotalUsage)
		systemDelta = float64(currentSystem) - float64(stats.PreCPUStats.SystemUsage)
	}

	if systemDelta > 0 && cpuDelta > 0 {
		cpuPct := (cpuDelta / systemDelta) * onlineCPUs * 100.0
		if cpuPct > onlineCPUs*100.0 {
			cpuPct = onlineCPUs * 100.0
		}
		info.CPUPct = math.Round(cpuPct*10) / 10
	} else {
		info.CPUPct = 0.0
	}

	// Memory usage: match Docker CLI by subtracting inactive_file / cache
	usedMem := stats.MemoryStats.Usage
	if statsMap := stats.MemoryStats.Stats; statsMap != nil {
		if usedMem == 0 {
			anon := parseUint64(statsMap["anon"])
			file := parseUint64(statsMap["file"])
			usedMem = anon + file
		}
		var cache uint64
		if v, ok := statsMap["inactive_file"]; ok {
			cache = parseUint64(v)
		} else if v, ok := statsMap["total_inactive_file"]; ok {
			cache = parseUint64(v)
		} else if v, ok := statsMap["cache"]; ok {
			cache = parseUint64(v)
		}
		if cache < usedMem {
			usedMem -= cache
		}
	}

	info.MemoryMB = math.Round((float64(usedMem)/(1024*1024))*10) / 10
	if stats.MemoryStats.Limit > 0 {
		pct := (float64(usedMem) / float64(stats.MemoryStats.Limit)) * 100.0
		info.MemoryPct = math.Round(pct*10) / 10
	}

	info.NetInputMB = 0
	info.NetOutputMB = 0
	for _, n := range stats.Networks {
		info.NetInputMB += float64(n.RxBytes) / (1024 * 1024)
		info.NetOutputMB += float64(n.TxBytes) / (1024 * 1024)
	}
	if len(stats.Networks) == 0 && (stats.Network.RxBytes > 0 || stats.Network.TxBytes > 0) {
		info.NetInputMB = float64(stats.Network.RxBytes) / (1024 * 1024)
		info.NetOutputMB = float64(stats.Network.TxBytes) / (1024 * 1024)
	}
	info.NetInputMB = math.Round(info.NetInputMB*10) / 10
	info.NetOutputMB = math.Round(info.NetOutputMB*10) / 10

	// Save to lastStats cache
	d.cpuMu.Lock()
	if d.lastStats == nil {
		d.lastStats = make(map[string]containerStatsSnapshot)
	}
	d.lastStats[id] = containerStatsSnapshot{
		CPUPct:      info.CPUPct,
		MemoryMB:    info.MemoryMB,
		MemoryPct:   info.MemoryPct,
		NetInputMB:  info.NetInputMB,
		NetOutputMB: info.NetOutputMB,
		UpdatedAt:   time.Now(),
	}
	d.cpuMu.Unlock()
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


func (d *SocketDriver) Close() error {
	return nil
}

func cleanDigest(d string) string {
	d = strings.TrimSpace(d)
	d = strings.Trim(d, "\"")
	if idx := strings.LastIndex(d, "sha256:"); idx != -1 {
		return d[idx+7:]
	}
	return strings.TrimPrefix(d, "sha256:")
}
