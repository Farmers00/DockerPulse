package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) handleListNetworks(c *gin.Context) {
	hostID := c.Param("id")
	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 6*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	nets, err := d.ListNetworks(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, nets)
}

func (s *Server) handleCreateNetwork(c *gin.Context) {
	hostID := c.Param("id")
	var req struct {
		Name   string `json:"name" binding:"required"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	if err := d.CreateNetwork(ctx, req.Name, req.Driver); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"success": true})
}

func (s *Server) handleRemoveNetwork(c *gin.Context) {
	hostID := c.Param("id")
	nid := c.Param("nid")

	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	if err := d.RemoveNetwork(ctx, nid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleConnectNetwork(c *gin.Context) {
	hostID := c.Param("id")
	nid := c.Param("nid")
	var req struct {
		ContainerID string `json:"container_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	if err := d.ConnectNetwork(ctx, nid, req.ContainerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleDisconnectNetwork(c *gin.Context) {
	hostID := c.Param("id")
	nid := c.Param("nid")
	var req struct {
		ContainerID string `json:"container_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	if err := d.DisconnectNetwork(ctx, nid, req.ContainerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleGetStorageUsage(c *gin.Context) {
	hostID := c.Param("id")
	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	usage, err := d.GetDiskUsage(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, usage)
}

func (s *Server) handlePruneStorage(c *gin.Context) {
	hostID := c.Param("id")
	pruneAll, _ := strconv.ParseBool(c.DefaultQuery("all", "false"))

	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 60*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	report, err := d.PruneResources(ctx, pruneAll)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

func (s *Server) handleCheckUpdates(c *gin.Context) {
	hostID := c.Param("id")
	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	results, err := s.updater.CheckHostContainers(ctx, d, hostID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, results)
}

func (s *Server) handleAgentWS(c *gin.Context) {
	token := c.Query("token")
	hostID := c.Query("host_id")

	if token != s.cfg.AgentSecret || hostID == "" {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	ws, err := s.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	session := s.agentManager.Register(hostID, ws)
	_ = s.db.UpdateHostStatus(hostID, "online")

	defer func() {
		s.agentManager.Unregister(hostID)
		_ = s.db.UpdateHostStatus(hostID, "offline")
		ws.Close()
	}()

	// Wait until connection closes
	for {
		if _, _, err := ws.ReadMessage(); err != nil {
			break
		}
	}
	_ = session
}

func (s *Server) handleGetAgentToken(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"token": s.cfg.AgentSecret,
	})
}

func (s *Server) handleDownloadBinary(c *gin.Context) {
	exePath, err := os.Executable()
	if err != nil {
		c.String(http.StatusInternalServerError, "Failed to resolve binary path: %v", err)
		return
	}

	// Verify file exists
	if _, err := os.Stat(exePath); err != nil {
		c.String(http.StatusNotFound, "Binary file not found on server")
		return
	}

	c.Header("Content-Disposition", "attachment; filename=dockerpulse")
	c.Header("Content-Type", "application/octet-stream")
	c.File(exePath)
}

func (s *Server) handleInstallAgentScript(c *gin.Context) {
	host := c.Request.Host
	scheme := "ws"
	httpScheme := "http"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "wss"
		httpScheme = "https"
	}

	token := c.DefaultQuery("token", s.cfg.AgentSecret)
	hostID := c.DefaultQuery("id", "remote-node")

	wsURL := fmt.Sprintf("%s://%s/ws/agent", scheme, host)
	serverURL := fmt.Sprintf("%s://%s", httpScheme, host)

	script := fmt.Sprintf(`#!/usr/bin/env bash
set -e

echo "======================================================"
echo "      DockerPulse Agent One-Line Installer"
echo "======================================================"

if ! command -v docker >/dev/null 2>&1; then
  echo "[Error] Docker is not installed on this host."
  echo "Please install Docker before running this installer."
  exit 1
fi

HOST_ID="%s"
TOKEN="%s"
WS_URL="%s"
SERVER_URL="%s"

ACTUAL_USER="$USER"
if [ "$EUID" -eq 0 ] && [ -n "$SUDO_USER" ]; then
  ACTUAL_USER="$SUDO_USER"
fi
USER_HOME=$(getent passwd "$ACTUAL_USER" 2>/dev/null | cut -d: -f6)
if [ -z "$USER_HOME" ]; then
  USER_HOME="$HOME"
fi
BASE_DIR="$USER_HOME/docker"

echo "[DockerPulse] Host ID:     $HOST_ID"
echo "[DockerPulse] Server URL:  $SERVER_URL"
echo "[DockerPulse] Base Dir:    $BASE_DIR"

mkdir -p "$BASE_DIR"

echo "[DockerPulse] Downloading agent binary from $SERVER_URL/download/dockerpulse..."
TEMP_BIN=$(mktemp)
if ! curl -fsSL "$SERVER_URL/download/dockerpulse" -o "$TEMP_BIN"; then
  echo "[Error] Failed to download agent binary from DockerPulse server."
  rm -f "$TEMP_BIN"
  exit 1
fi
chmod +x "$TEMP_BIN"

INSTALL_BIN="/usr/local/bin/dockerpulse"
SUDO=""
if [ "$EUID" -ne 0 ]; then
  if command -v sudo >/dev/null 2>&1; then
    SUDO="sudo"
  else
    INSTALL_BIN="$USER_HOME/.local/bin/dockerpulse"
    mkdir -p "$USER_HOME/.local/bin"
  fi
fi

echo "[DockerPulse] Installing binary to $INSTALL_BIN..."
$SUDO mv "$TEMP_BIN" "$INSTALL_BIN"
$SUDO chmod 755 "$INSTALL_BIN"

# Configure systemd service if available
if command -v systemctl >/dev/null 2>&1 && [ -d /etc/systemd/system ]; then
  echo "[DockerPulse] Setting up systemd service: dockerpulse-agent.service..."
  SERVICE_FILE="/tmp/dockerpulse-agent.service"
  cat << EOF > "$SERVICE_FILE"
[Unit]
Description=DockerPulse Remote Agent
After=docker.service network.target
Requires=docker.service

[Service]
Type=simple
User=root
ExecStart=$INSTALL_BIN agent --server $WS_URL --token $TOKEN --host-id $HOST_ID --base-dir $BASE_DIR
Restart=always
RestartSec=5s
Environment=PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

[Install]
WantedBy=multi-user.target
EOF

  $SUDO mv "$SERVICE_FILE" /etc/systemd/system/dockerpulse-agent.service
  $SUDO systemctl daemon-reload
  $SUDO systemctl enable --now dockerpulse-agent
  echo "[DockerPulse] Agent service started successfully via systemd!"
else
  echo "[DockerPulse] Starting agent in background..."
  nohup $INSTALL_BIN agent --server "$WS_URL" --token "$TOKEN" --host-id "$HOST_ID" --base-dir "$BASE_DIR" > "$USER_HOME/dockerpulse-agent.log" 2>&1 &
  echo "[DockerPulse] Agent running in background (PID $!)"
fi

echo ""
echo "======================================================"
echo " DockerPulse Agent is now running!"
echo " Host '$HOST_ID' connected to $SERVER_URL"
echo "======================================================"
`, hostID, token, wsURL, serverURL, hostID)

	c.Header("Content-Type", "text/x-shellscript; charset=utf-8")
	c.String(http.StatusOK, script)
}

func (s *Server) handleAgentComposeTemplate(c *gin.Context) {
	host := c.Request.Host
	scheme := "ws"
	if c.Request.TLS != nil || c.GetHeader("X-Forwarded-Proto") == "https" {
		scheme = "wss"
	}

	token := c.DefaultQuery("token", s.cfg.AgentSecret)
	hostID := c.DefaultQuery("id", "node-1")
	wsURL := fmt.Sprintf("%s://%s/ws/agent", scheme, host)

	template := fmt.Sprintf(`services:
  dockerpulse-agent:
    image: dockerpulse-agent:local
    build:
      context: ..
      dockerfile: deploy/Dockerfile
    container_name: dockerpulse-agent
    restart: unless-stopped
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock
      - ${HOME}/docker:/root/docker
    command: >
      dockerpulse agent
      --server %s
      --token %s
      --host-id %s
      --base-dir /root/docker
`, wsURL, token, hostID)

	c.Header("Content-Type", "text/yaml; charset=utf-8")
	c.String(http.StatusOK, template)
}
