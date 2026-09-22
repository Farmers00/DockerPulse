package driver

import (
	"context"
	"io"
	"time"
)

type SystemInfo struct {
	Hostname      string  `json:"hostname"`
	OS            string  `json:"os"`
	KernelVersion string  `json:"kernel_version"`
	DockerVersion string  `json:"docker_version"`
	TotalCPUs     int     `json:"total_cpus"`
	CPUUsagePct   float64 `json:"cpu_usage_pct"`
	TotalRAMBytes uint64  `json:"total_ram_bytes"`
	UsedRAMBytes  uint64  `json:"used_ram_bytes"`
	DiskTotalGB   float64 `json:"disk_total_gb"`
	DiskFreeGB    float64 `json:"disk_free_gb"`
}

type ContainerPort struct {
	IP          string `json:"ip,omitempty"`
	PrivatePort uint16 `json:"private_port"`
	PublicPort  uint16 `json:"public_port,omitempty"`
	Type        string `json:"type"`
}

type ContainerInfo struct {
	ID         string          `json:"id"`
	Names      []string        `json:"names"`
	Image      string          `json:"image"`
	ImageID    string          `json:"image_id"`
	Command    string          `json:"command"`
	Created    int64           `json:"created"`
	State      string          `json:"state"` // "running", "exited", "paused"
	Status     string          `json:"status"` // e.g. "Up 2 hours"
	Ports      []ContainerPort `json:"ports"`
	Stack      string          `json:"stack,omitempty"` // Derived from com.docker.compose.project label
	Service    string          `json:"service,omitempty"` // Derived from com.docker.compose.service label
	WorkingDir string          `json:"working_dir,omitempty"` // com.docker.compose.project.working_dir
	ConfigFile string          `json:"config_file,omitempty"` // com.docker.compose.project.config_files
	CPUPct     float64         `json:"cpu_pct"`
	MemoryMB   float64         `json:"memory_mb"`
	MemoryPct  float64         `json:"memory_pct"`
	NetInputMB float64         `json:"net_input_mb"`
	NetOutputMB float64        `json:"net_output_mb"`
	HasUpdate  bool            `json:"has_update"`
}

type NetworkInfo struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Driver     string            `json:"driver"`
	Scope      string            `json:"scope"`
	Internal   bool              `json:"internal"`
	Subnet     string            `json:"subnet,omitempty"`
	Gateway    string            `json:"gateway,omitempty"`
	Containers map[string]string `json:"containers"` // ContainerID -> IPv4
}

type DiskUsageInfo struct {
	LayersSize     int64 `json:"layers_size"`
	ImagesSize     int64 `json:"images_size"`
	ContainersSize int64 `json:"containers_size"`
	VolumesSize    int64 `json:"volumes_size"`
	BuildCacheSize int64 `json:"build_cache_size"`
	DanglingImages int   `json:"dangling_images"`
	UnusedVolumes  int   `json:"unused_volumes"`
}

type PruneReport struct {
	ImagesDeleted  int   `json:"images_deleted"`
	SpaceReclaimed int64 `json:"space_reclaimed"`
}

type DiscoveredStack struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	ComposeFile string    `json:"compose_file"`
	HasEnvFile  bool      `json:"has_env_file"`
	UpdatedAt   time.Time `json:"updated_at"`
	Services    []string  `json:"services"`
}

type TerminalSize struct {
	Rows uint16 `json:"rows"`
	Cols uint16 `json:"cols"`
}

type HostDriver interface {
	Ping(ctx context.Context) error
	GetSystemInfo(ctx context.Context) (*SystemInfo, error)
	ListContainers(ctx context.Context) ([]ContainerInfo, error)
	StartContainer(ctx context.Context, id string) error
	StopContainer(ctx context.Context, id string) error
	RestartContainer(ctx context.Context, id string) error
	RemoveContainer(ctx context.Context, id string, force bool) error
	StreamLogs(ctx context.Context, containerID string, follow bool, tail string, writer io.Writer) error
	ExecShell(ctx context.Context, containerID string, cmd []string, in io.Reader, out io.Writer, resizeChan <-chan TerminalSize) error
	ListNetworks(ctx context.Context) ([]NetworkInfo, error)
	CreateNetwork(ctx context.Context, name, driver string) error
	RemoveNetwork(ctx context.Context, id string) error
	ConnectNetwork(ctx context.Context, netID, containerID string) error
	DisconnectNetwork(ctx context.Context, netID, containerID string) error
	GetDiskUsage(ctx context.Context) (*DiskUsageInfo, error)
	PruneResources(ctx context.Context, pruneAll bool) (*PruneReport, error)
	DiscoverStacks(ctx context.Context, baseDir string) ([]DiscoveredStack, error)
	ReadStackFiles(ctx context.Context, stackPath string) (composeContent string, envContent string, err error)
	WriteStackFiles(ctx context.Context, stackPath string, composeContent string, envContent string) error
	ExecuteCompose(ctx context.Context, stackPath string, action string, writer io.Writer) error
	Close() error
}
