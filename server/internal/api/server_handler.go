package api

import (
	"database/sql"
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

	sourceMap := h.loadSourceMap(h.store.DB())
	for i := range servers {
		if src, ok := sourceMap[servers[i].HostID]; ok {
			servers[i].Source = src
		} else {
			servers[i].Source = "agent"
		}
	}

	c.JSON(http.StatusOK, servers)
}

// loadSourceMap batch-loads source info for all servers in two queries instead of 2*N.
func (h *ServerHandler) loadSourceMap(db *sql.DB) map[string]string {
	result := make(map[string]string)

	// managed_servers: agent_host_id -> "managed"
	rows, err := db.Query("SELECT agent_host_id FROM managed_servers WHERE agent_host_id != ''")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var hostID string
			if rows.Scan(&hostID) == nil {
				result[hostID] = "managed"
			}
		}
	}

	// cloud_instances (ECS only): host_id -> "cloud" (only if not already "managed")
	rows2, err := db.Query("SELECT host_id FROM cloud_instances WHERE instance_type = 'ecs'")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var hostID string
			if rows2.Scan(&hostID) == nil {
				if _, exists := result[hostID]; !exists {
					result[hostID] = "cloud"
				}
			}
		}
	}

	return result
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
