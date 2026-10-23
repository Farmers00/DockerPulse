package driver

import (
	"context"
	"fmt"
)

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

	return report, nil
}

