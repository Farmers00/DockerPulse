package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
		if hostBase == "/root/docker" || hostBase == "" {
			if envHost := os.Getenv("HOST_BASE_DIR"); envHost != "" {
				hostBase = envHost
			} else {
				hostBase = "~/docker"
			}
		}
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

	// Build lookup sets for all discovered stacks
	discoveredNames := make(map[string]bool)
	discoveredPaths := make(map[string]bool)
	for _, disc := range discovered {
		nameLower := strings.ToLower(strings.TrimSpace(disc.Name))
		if nameLower != "" {
			discoveredNames[nameLower] = true
		}
		if disc.Path != "" {
			discoveredPaths[cleanStackPath(disc.Path)] = true
		}

		discPath := disc.Path
		if strings.HasPrefix(discPath, "/root/docker") {
			if host.BaseDir != "" && host.BaseDir != "/root/docker" {
				discPath = strings.Replace(discPath, "/root/docker", host.BaseDir, 1)
			} else {
				discPath = strings.Replace(discPath, "/root/docker", "~/docker", 1)
			}
			discoveredPaths[cleanStackPath(discPath)] = true
		}

		stack := &database.Stack{
			HostID: hostID,
			Name:   disc.Name,
			Path:   discPath,
			Status: "discovered",
		}
		_ = s.db.UpsertStack(stack)

		// Snapshot compose and env files if readable to keep revisions up to date
		if composeContent, envContent, err := d.ReadStackFiles(ctx, stack.Path); err == nil && composeContent != "" {
			revs, _ := s.db.ListRevisions(stack.ID)
			if len(revs) == 0 {
				_ = s.db.CreateRevision(&database.StackRevision{
					StackID:        stack.ID,
					ComposeContent: composeContent,
					EnvContent:     envContent,
					CreatedBy:      "system",
					Note:           "Initial scan snapshot",
				})
			} else {
				latest := revs[0]
				if latest.ComposeContent != composeContent || latest.EnvContent != envContent {
					_ = s.db.CreateRevision(&database.StackRevision{
						StackID:        stack.ID,
						ComposeContent: composeContent,
						EnvContent:     envContent,
						CreatedBy:      "system",
						Note:           "Auto-synced from host directory",
					})
				}
			}
		}
	}

	// Purge stale duplicate /root/docker stacks and flag deleted stacks that no longer exist on host
	existing, _ := s.db.ListStacks(hostID)
	for _, st := range existing {
		// 1. Purge stale duplicate /root/docker stacks if another stack with the same name and a real host path exists
		if strings.HasPrefix(st.Path, "/root/docker/") {
			hasRealHostPath := false
			for _, other := range existing {
				if other.ID != st.ID && strings.EqualFold(other.Name, st.Name) && !strings.HasPrefix(other.Path, "/root/docker/") {
					hasRealHostPath = true
					break
				}
			}
			if hasRealHostPath {
				_ = s.db.DeleteStack(st.ID)
				continue
			}
		}

		// 2. If stack was not found during discovery (neither in directory nor as an active/stopped container)
		stNameLower := strings.ToLower(strings.TrimSpace(st.Name))
		stCleanPath := cleanStackPath(st.Path)
		if !discoveredNames[stNameLower] && !discoveredPaths[stCleanPath] {
			// Verify if the stack files are actually gone
			if _, _, err := d.ReadStackFiles(ctx, st.Path); err != nil {
				// Stack folder is missing from host disk. Instead of deleting, mark as "missing" so user can restore it
				st.Status = "missing"
				_ = s.db.UpdateStackStatus(st.ID, "missing")
			}
		}
	}

	stacks, _ := s.db.ListStacks(hostID)
	c.JSON(http.StatusOK, stacks)
}

func cleanStackPath(p string) string {
	if p == "" {
		return ""
	}
	p = strings.TrimSpace(p)
	p = strings.ReplaceAll(p, "\\", "/")
	p = filepath.Clean(p)
	return strings.ReplaceAll(p, "\\", "/")
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
		// Fallback to latest revision in database if files cannot be read from host (e.g. missing stack)
		revs, revErr := s.db.ListRevisions(stack.ID)
		if revErr == nil && len(revs) > 0 {
			c.JSON(http.StatusOK, gin.H{
				"compose": revs[0].ComposeContent,
				"env":     revs[0].EnvContent,
				"path":    stack.Path,
				"cached":  true,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"compose": composeContent,
		"env":     envContent,
		"path":    stack.Path,
	})
}

func (s *Server) handleRestoreStack(c *gin.Context) {
	hostID := c.Param("id")
	stackID := c.Param("sid")

	var req struct {
		Path       string `json:"path"`
		RevisionID string `json:"revision_id"`
		Start      bool   `json:"start"`
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

	targetPath := strings.TrimSpace(req.Path)
	if targetPath == "" {
		targetPath = stack.Path
	}

	// Fetch revision to restore
	var rev *database.StackRevision
	if req.RevisionID != "" {
		rev, err = s.db.GetRevision(req.RevisionID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Specified revision not found"})
			return
		}
	} else {
		revs, err := s.db.ListRevisions(stack.ID)
		if err != nil || len(revs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "No saved revisions available to restore this stack"})
			return
		}
		rev = &revs[0]
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	// Write files back to target host directory
	if err := d.WriteStackFiles(ctx, targetPath, rev.ComposeContent, rev.EnvContent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to write stack files: %v", err)})
		return
	}

	// Update stack path and status back to normal in DB
	_ = s.db.UpdateStackPathAndStatus(stack.ID, targetPath, "stopped")
	stack.Path = targetPath
	stack.Status = "stopped"

	// Create revision note for restore
	_ = s.db.CreateRevision(&database.StackRevision{
		StackID:        stack.ID,
		ComposeContent: rev.ComposeContent,
		EnvContent:     rev.EnvContent,
		CreatedBy:      "restore",
		Note:           fmt.Sprintf("Restored to %s from revision #%d", targetPath, rev.RevisionNum),
	})

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"stack":   stack,
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
		if err := d.ExecuteCompose(c.Request.Context(), stack.Path, req.Action, pipeW); err != nil {
			fmt.Fprintf(pipeW, "\n[DockerPulse] Command finished with error: %v\n", err)
		}
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
