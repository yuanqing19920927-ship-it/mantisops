package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"opsboard/server/internal/store"
)

type ServerHandler struct {
	store *store.ServerStore
}

func (h *ServerHandler) List(c *gin.Context) {
	servers, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Determine source for each server
	for i := range servers {
		servers[i].Source = h.determineSource(servers[i].HostID)
	}

	c.JSON(http.StatusOK, servers)
}

func (h *ServerHandler) determineSource(hostID string) string {
	var count int
	// Check managed_servers first
	h.store.DB().QueryRow("SELECT COUNT(*) FROM managed_servers WHERE agent_host_id = ?", hostID).Scan(&count)
	if count > 0 {
		return "managed"
	}
	// Check cloud_instances
	h.store.DB().QueryRow("SELECT COUNT(*) FROM cloud_instances WHERE host_id = ? AND instance_type = 'ecs'", hostID).Scan(&count)
	if count > 0 {
		return "cloud"
	}
	return "agent"
}

func (h *ServerHandler) Get(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.store.GetByHostID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusOK, srv)
}

func (h *ServerHandler) UpdateDisplayName(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		DisplayName string `json:"display_name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.store.UpdateDisplayName(id, req.DisplayName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
