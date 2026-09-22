package api

import (
	"context"
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
