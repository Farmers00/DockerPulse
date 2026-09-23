package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dockpulse/dockmgr/internal/database"
	"github.com/dockpulse/dockmgr/internal/driver"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleListStacks(c *gin.Context) {
	hostID := c.Param("id")
	stacks, err := s.db.ListStacks(hostID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stacks)
}

func (s *Server) handleDiscoverStacks(c *gin.Context) {
	hostID := c.Param("id")
	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	// If host.BaseDir is set to a container path (e.g. /root/docker), translate to host path
	if socketDriver, ok := d.(*driver.SocketDriver); ok {
		hostBase := socketDriver.ToHostPath(ctx, host.BaseDir)
		if hostBase != "" && hostBase != host.BaseDir {
			host.BaseDir = hostBase
			_ = s.db.UpdateHost(host)
		}
	}

	discovered, err := d.DiscoverStacks(ctx, host.BaseDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Upsert discovered into database
	discoveredNames := make(map[string]bool)
	for _, disc := range discovered {
		discoveredNames[disc.Name] = true
		stack := &database.Stack{
			HostID: hostID,
			Name:   disc.Name,
			Path:   disc.Path,
			Status: "discovered",
		}
		_ = s.db.UpsertStack(stack)
	}

	// Purge any stale duplicate stacks for this host (e.g. leftover /root/docker stacks)
	existing, _ := s.db.ListStacks(hostID)
	for _, st := range existing {
		if strings.HasPrefix(st.Path, "/root/docker/") && discoveredNames[st.Name] {
			_ = s.db.DeleteStack(st.ID)
		}
	}

	stacks, _ := s.db.ListStacks(hostID)
	c.JSON(http.StatusOK, stacks)
}

func (s *Server) handleDeleteStack(c *gin.Context) {
	stackID := c.Param("sid")
	if err := s.db.DeleteStack(stackID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleGetStackFiles(c *gin.Context) {
	hostID := c.Param("id")
	stackID := c.Param("sid")

	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	stack, err := s.db.GetStack(stackID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Stack not found"})
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

	composeContent, envContent, err := d.ReadStackFiles(ctx, stack.Path)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"compose": composeContent,
		"env":     envContent,
		"path":    stack.Path,
	})
}

func (s *Server) handleSaveStackFiles(c *gin.Context) {
	hostID := c.Param("id")
	stackID := c.Param("sid")

	host, err := s.db.GetHost(hostID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	stack, err := s.db.GetStack(stackID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Stack not found"})
		return
	}

	var req struct {
		Compose string `json:"compose" binding:"required"`
		Env     string `json:"env"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	// 1. Create a revision backup of existing content before overwriting
	oldCompose, oldEnv, _ := d.ReadStackFiles(ctx, stack.Path)
	if oldCompose != "" {
		username, _ := c.Get("username")
		_ = s.db.CreateRevision(&database.StackRevision{
			StackID:        stack.ID,
			ComposeContent: oldCompose,
			EnvContent:     oldEnv,
			CreatedBy:      fmt.Sprintf("%v", username),
			Note:           req.Note,
		})
	}

	// 2. Write new files to the host
	if err := d.WriteStackFiles(ctx, stack.Path, req.Compose, req.Env); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (s *Server) handleComposeAction(c *gin.Context) {
	hostID := c.Param("id")
	stackID := c.Param("sid")

	var req struct {
		Action string `json:"action" binding:"required"` // "up", "down", "pull", "restart", "pull_up"
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

	stack, err := s.db.GetStack(stackID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Stack not found"})
		return
	}

	d, err := s.GetDriver(c.Request.Context(), host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	// Stream execution logs back as Server-Sent Events / plain streaming chunk
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Transfer-Encoding", "chunked")
	c.Header("X-Content-Type-Options", "nosniff")

	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	pipeR, pipeW := io.Pipe()
	go func() {
		defer pipeW.Close()
		_ = d.ExecuteCompose(c.Request.Context(), stack.Path, req.Action, pipeW)
	}()

	buf := make([]byte, 1024)
	for {
		n, err := pipeR.Read(buf)
		if n > 0 {
			_, _ = c.Writer.Write(buf[:n])
			c.Writer.Flush()
		}
		if err != nil {
			break
		}
	}
}

func (s *Server) handleListRevisions(c *gin.Context) {
	stackID := c.Param("sid")
	revs, err := s.db.ListRevisions(stackID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, revs)
}
