package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"opsboard/server/internal/store"
)

type DashboardHandler struct {
	serverStore *store.ServerStore
}

func (h *DashboardHandler) Overview(c *gin.Context) {
	servers, err := h.serverStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	online, total := 0, len(servers)
	for _, s := range servers {
		if s.Status == "online" {
			online++
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"servers_online": online,
		"servers_total":  total,
		"servers":        servers,
	})
}
