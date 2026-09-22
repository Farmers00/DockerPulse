//go:build windows

package driver

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"
)

const defaultDockerSocket = `\\.\pipe\docker_engine`

func dialDockerSocket(ctx context.Context, socketPath string) (net.Conn, error) {
	if strings.HasPrefix(socketPath, `\\.\pipe\`) || strings.HasPrefix(socketPath, `npipe://`) {
		// Attempt standard localhost fallback or pipe
		var d net.Dialer
		conn, err := d.DialContext(ctx, "tcp", "127.0.0.1:2375")
		if err == nil {
			return conn, nil
		}
		return nil, errors.New("Docker named pipe on Windows requires TCP daemon or running inside Docker container")
	}

	var d net.Dialer
	d.Timeout = 5 * time.Second
	return d.DialContext(ctx, "tcp", socketPath)
}
