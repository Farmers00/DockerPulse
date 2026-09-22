//go:build !windows

package driver

import (
	"context"
	"net"
)

const defaultDockerSocket = "/var/run/docker.sock"

func dialDockerSocket(ctx context.Context, socketPath string) (net.Conn, error) {
	var d net.Dialer
	return d.DialContext(ctx, "unix", socketPath)
}
