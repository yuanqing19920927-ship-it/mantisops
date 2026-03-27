package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/store"
	pb "mantisops/server/proto/gen"
)

// MetricsProvider 提供缓存的指标快照
type MetricsProvider interface {
	GetCachedMetrics() map[string]*pb.MetricsPayload
}

type DashboardHandler struct {
	serverStore     *store.ServerStore
	metricsProvider MetricsProvider
	groupStore      *store.GroupStore
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

	resp := gin.H{
		"servers_online": online,
		"servers_total":  total,
		"servers":        servers,
	}

	if h.groupStore != nil {
		groups, _ := h.groupStore.ListSimple()
		resp["groups"] = groups
	}

	// 附带缓存的指标快照，前端无需等 WebSocket 即可显示最新数据
	if h.metricsProvider != nil {
		resp["metrics"] = h.metricsProvider.GetCachedMetrics()
	}

	c.JSON(http.StatusOK, resp)
}
