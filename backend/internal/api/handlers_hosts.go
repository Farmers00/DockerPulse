package api

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/dockpulse/dockmgr/internal/database"
	"github.com/gin-gonic/gin"
)

func (s *Server) handleListHosts(c *gin.Context) {
	hosts, err := s.db.ListHosts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Concurrently ping each host to update status
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	for i := range hosts {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			h := &hosts[idx]
			d, err := s.GetDriver(ctx, h)
			if err != nil {
				h.Status = "offline"
				return
			}
			defer d.Close()

			if err := d.Ping(ctx); err != nil {
				h.Status = "offline"
			} else {
				h.Status = "online"
				_ = s.db.UpdateHostStatus(h.ID, "online")
			}
		}(i)
	}
	wg.Wait()

	c.JSON(http.StatusOK, hosts)
}

func (s *Server) handleCreateHost(c *gin.Context) {
	var req struct {
		Name      string              `json:"name" binding:"required"`
		Driver    database.DriverType `json:"driver" binding:"required"`
		Address   string              `json:"address"`
		Port      int                 `json:"port"`
		AuthToken string              `json:"auth_token"`
		SSHUser   string              `json:"ssh_user"`
		SSHKey    string              `json:"ssh_key"`
		BaseDir   string              `json:"base_dir"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.BaseDir == "" {
		req.BaseDir = "~/docker"
	}

	host := &database.Host{
		Name:      req.Name,
		Driver:    req.Driver,
		Address:   req.Address,
		Port:      req.Port,
		AuthToken: req.AuthToken,
		SSHUser:   req.SSHUser,
		SSHKey:    req.SSHKey,
		BaseDir:   req.BaseDir,
		Status:    "unknown",
	}

	if err := s.db.CreateHost(host); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, host)
}

func (s *Server) handleGetHost(c *gin.Context) {
	id := c.Param("id")
	host, err := s.db.GetHost(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}
	c.JSON(http.StatusOK, host)
}

func (s *Server) handleDeleteHost(c *gin.Context) {
	id := c.Param("id")
	if err := s.db.DeleteHost(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Host deleted"})
}

func (s *Server) handleGetSystemInfo(c *gin.Context) {
	id := c.Param("id")
	host, err := s.db.GetHost(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Host not found"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	d, err := s.GetDriver(ctx, host)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer d.Close()

	info, err := d.GetSystemInfo(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, info)
}
