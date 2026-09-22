package api

import (
	"context"
	"fmt"
	"net/http"
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
  echo "Please install Docker and Docker Compose before running this installer."
  exit 1
fi

HOST_ID="%s"
TOKEN="%s"
WS_URL="%s"
SERVER_URL="%s"
TARGET_DIR="$HOME/docker/dockerpulse-agent"

echo "[DockerPulse] Setting up agent directory: $TARGET_DIR"
mkdir -p "$TARGET_DIR"
cd "$TARGET_DIR"

echo "[DockerPulse] Writing docker-compose.yml..."
cat << 'EOF' > docker-compose.yml
services:
  dockerpulse-agent:
    image: dockerpulse/dockerpulse:latest
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
EOF

echo "[DockerPulse] Starting DockerPulse agent..."
docker compose up -d

echo ""
echo "======================================================"
echo " DockerPulse Agent is now running!"
echo " Host '%s' registered with: $SERVER_URL"
echo "======================================================"
`, hostID, token, wsURL, serverURL, wsURL, token, hostID, hostID)

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
    image: dockerpulse/dockerpulse:latest
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
