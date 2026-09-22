package driver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type SocketDriver struct {
	client *DockerClient
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

	result := make([]ContainerInfo, 0, len(raw))
	for _, c := range raw {
		info := ContainerInfo{
			ID:         c.ID,
			Names:      c.Names,
			Image:      c.Image,
			ImageID:    c.ImageID,
			Command:    c.Command,
			Created:    c.Created,
			State:      c.State,
			Status:     c.Status,
			Stack:      c.Labels["com.docker.compose.project"],
			Service:    c.Labels["com.docker.compose.service"],
			WorkingDir: c.Labels["com.docker.compose.project.working_dir"],
			ConfigFile: c.Labels["com.docker.compose.project.config_files"],
		}

		for _, p := range c.Ports {
			info.Ports = append(info.Ports, ContainerPort{
				IP:          p.IP,
				PrivatePort: p.PrivatePort,
				PublicPort:  p.PublicPort,
				Type:        p.Type,
			})
		}

		// Quick stats if running
		if c.State == "running" {
			d.populateContainerStats(ctx, c.ID, &info)
		}

		result = append(result, info)
	}
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

// Stacks filesystem discovery and compose execution
func (d *SocketDriver) DiscoverStacks(ctx context.Context, baseDir string) ([]DiscoveredStack, error) {
	expandedPath := expandHomeDir(baseDir)
	entries, err := os.ReadDir(expandedPath)
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

		dirPath := filepath.Join(expandedPath, entry.Name())
		composeFile := findComposeFile(dirPath)
		if composeFile != "" {
			info, _ := entry.Info()
			hasEnv := false
			if _, err := os.Stat(filepath.Join(dirPath, ".env")); err == nil {
				hasEnv = true
			}

			stacks = append(stacks, DiscoveredStack{
				Name:        entry.Name(),
				Path:        dirPath,
				ComposeFile: filepath.Base(composeFile),
				HasEnvFile:  hasEnv,
				UpdatedAt:   info.ModTime(),
			})
		}
	}
	return stacks, nil
}

func (d *SocketDriver) ReadStackFiles(ctx context.Context, stackPath string) (string, string, error) {
	composeFile := findComposeFile(stackPath)
	if composeFile == "" {
		return "", "", fmt.Errorf("no compose file found in %s", stackPath)
	}

	composeBytes, err := os.ReadFile(composeFile)
	if err != nil {
		return "", "", fmt.Errorf("failed to read compose file: %w", err)
	}

	envBytes, _ := os.ReadFile(filepath.Join(stackPath, ".env"))
	return string(composeBytes), string(envBytes), nil
}

func (d *SocketDriver) WriteStackFiles(ctx context.Context, stackPath string, composeContent string, envContent string) error {
	composeFile := findComposeFile(stackPath)
	if composeFile == "" {
		composeFile = filepath.Join(stackPath, "docker-compose.yml")
	}

	if err := os.MkdirAll(stackPath, 0755); err != nil {
		return err
	}

	if err := os.WriteFile(composeFile, []byte(composeContent), 0644); err != nil {
		return fmt.Errorf("failed to write compose file: %w", err)
	}

	if envContent != "" || fileExists(filepath.Join(stackPath, ".env")) {
		if err := os.WriteFile(filepath.Join(stackPath, ".env"), []byte(envContent), 0644); err != nil {
			return fmt.Errorf("failed to write .env file: %w", err)
		}
	}

	return nil
}

func (d *SocketDriver) ExecuteCompose(ctx context.Context, stackPath string, action string, writer io.Writer) error {
	var args []string
	switch action {
	case "up":
		args = []string{"compose", "up", "-d"}
	case "down":
		args = []string{"compose", "down"}
	case "pull":
		args = []string{"compose", "pull"}
	case "restart":
		args = []string{"compose", "restart"}
	case "pull_up":
		if err := d.ExecuteCompose(ctx, stackPath, "pull", writer); err != nil {
			return err
		}
		return d.ExecuteCompose(ctx, stackPath, "up", writer)
	default:
		return fmt.Errorf("unsupported compose action: %s", action)
	}

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = stackPath
	cmd.Stdout = writer
	cmd.Stderr = writer

	fmt.Fprintf(writer, "[DockPulse] Running: docker %s (in %s)\n", strings.Join(args, " "), stackPath)
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
