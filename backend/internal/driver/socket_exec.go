package driver

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

func (d *SocketDriver) ExecShell(ctx context.Context, containerID string, cmd []string, in io.Reader, out io.Writer, resizeChan <-chan TerminalSize) error {
	// Candidate shells to try if using default shell
	var shellCandidates [][]string
	if len(cmd) > 0 && !(len(cmd) == 1 && cmd[0] == "/bin/sh") {
		shellCandidates = [][]string{cmd}
	} else {
		shellCandidates = [][]string{
			{"/bin/sh"},
			{"/bin/bash"},
			{"sh"},
		}
	}

	var lastErr error
	for _, candidateCmd := range shellCandidates {
		err := d.runExec(ctx, containerID, candidateCmd, in, out, resizeChan)
		if err == nil {
			return nil
		}
		lastErr = err
		// If error is not executable not found, don't keep retrying other shells
		errLower := strings.ToLower(err.Error())
		if !strings.Contains(errLower, "executable file not found") && !strings.Contains(errLower, "no such file") {
			break
		}
	}

	if lastErr != nil {
		errLower := strings.ToLower(lastErr.Error())
		if strings.Contains(errLower, "executable file not found") || strings.Contains(errLower, "no such file") {
			_, _ = fmt.Fprintf(out, "\r\n\x1b[33m[DockerPulse] Notice: No shell (/bin/sh or /bin/bash) found in container.\x1b[0m\r\n\x1b[90mThis container is likely built 'FROM scratch' (such as Watchtower or static Go/Rust binaries) and does not have an interactive shell environment.\x1b[0m\r\n")
		}
	}
	return lastErr
}

func (d *SocketDriver) runExec(ctx context.Context, containerID string, cmd []string, in io.Reader, out io.Writer, resizeChan <-chan TerminalSize) error {
	// 1. Try native Docker Engine API exec first (handles PTY/TTY properly over socket/TCP)
	err := d.runDockerAPIExec(ctx, containerID, cmd, in, out, resizeChan)
	if err == nil {
		return nil
	}

	// If the shell binary does not exist in the container, do not attempt fallback
	errLower := strings.ToLower(err.Error())
	if strings.Contains(errLower, "executable file not found") || strings.Contains(errLower, "no such file") {
		return err
	}

	// 2. Subprocess fallback using 'docker exec -i' (WITHOUT -t to avoid "the input device is not a TTY")
	args := append([]string{"exec", "-i", containerID}, cmd...)
	execCmd := exec.CommandContext(ctx, "docker", args...)
	execCmd.Stdin = in
	execCmd.Stdout = out
	execCmd.Stderr = out
	return execCmd.Run()
}

func (d *SocketDriver) runDockerAPIExec(ctx context.Context, containerID string, cmd []string, in io.Reader, out io.Writer, resizeChan <-chan TerminalSize) error {
	type execConfig struct {
		AttachStdin  bool     `json:"AttachStdin"`
		AttachStdout bool     `json:"AttachStdout"`
		AttachStderr bool     `json:"AttachStderr"`
		Tty          bool     `json:"Tty"`
		Cmd          []string `json:"Cmd"`
	}

	var execResp struct {
		ID string `json:"Id"`
	}

	createPayload := execConfig{
		AttachStdin:  true,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          true,
		Cmd:          cmd,
	}

	if err := d.client.Post(ctx, fmt.Sprintf("/containers/%s/exec", containerID), createPayload, &execResp); err != nil {
		return fmt.Errorf("failed to create exec: %w", err)
	}

	execID := execResp.ID

	// Dial Docker daemon directly for raw hijacked stream
	conn, err := d.client.Dial(ctx)
	if err != nil {
		return fmt.Errorf("failed to dial docker socket: %w", err)
	}
	defer conn.Close()

	reqBody := `{"Detach":false,"Tty":true}`
	reqStr := fmt.Sprintf("POST /exec/%s/start HTTP/1.1\r\n"+
		"Host: docker\r\n"+
		"User-Agent: Docker-Client\r\n"+
		"Content-Type: application/json\r\n"+
		"Connection: Upgrade\r\n"+
		"Upgrade: tcp\r\n"+
		"Content-Length: %d\r\n\r\n%s", execID, len(reqBody), reqBody)

	if _, err := conn.Write([]byte(reqStr)); err != nil {
		return fmt.Errorf("failed to send exec start request: %w", err)
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: "POST"})
	if err != nil {
		return fmt.Errorf("failed to read exec start response: %w", err)
	}

	if resp.StatusCode != http.StatusSwitchingProtocols && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("exec start returned status %d: %s", resp.StatusCode, string(b))
	}

	// Handle PTY resize events in background
	resizeCtx, cancelResize := context.WithCancel(ctx)
	defer cancelResize()
	go func() {
		for {
			select {
			case <-resizeCtx.Done():
				return
			case size, ok := <-resizeChan:
				if !ok {
					return
				}
				if size.Rows > 0 && size.Cols > 0 {
					_ = d.client.Post(resizeCtx, fmt.Sprintf("/exec/%s/resize?h=%d&w=%d", execID, size.Rows, size.Cols), nil, nil)
				}
			}
		}
	}()

	// Probe initial output with short read deadline to detect immediate exit / missing executable
	initialBuf := make([]byte, 512)
	_ = conn.SetReadDeadline(time.Now().Add(120 * time.Millisecond))
	n, readErr := br.Read(initialBuf)
	_ = conn.SetReadDeadline(time.Time{})

	if n > 0 {
		initialOutput := string(initialBuf[:n])
		lower := strings.ToLower(initialOutput)
		if strings.Contains(lower, "executable file not found") || strings.Contains(lower, "no such file") {
			return errors.New(strings.TrimSpace(initialOutput))
		}
		// Write valid initial output to terminal
		if _, err := out.Write(initialBuf[:n]); err != nil {
			return err
		}
	} else if readErr != nil && !errors.Is(readErr, os.ErrDeadlineExceeded) && !strings.Contains(readErr.Error(), "timeout") {
		// Connection closed immediately; inspect exit code
		var inspectResp struct {
			ExitCode int  `json:"ExitCode"`
			Running  bool `json:"Running"`
		}
		if d.client.Get(ctx, fmt.Sprintf("/exec/%s/json", execID), &inspectResp) == nil {
			if inspectResp.ExitCode == 126 || inspectResp.ExitCode == 127 {
				return fmt.Errorf("executable file not found (exit code %d)", inspectResp.ExitCode)
			}
		}
	}

	// Bidirectional stream copying
	done := make(chan struct{})

	// Stream stdout/stderr from container PTY to WebSocket
	go func() {
		_, _ = io.Copy(out, br)
		close(done)
	}()

	// Stream stdin from WebSocket to container PTY
	go func() {
		_, _ = io.Copy(conn, in)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-done:
		return nil
	}
}

