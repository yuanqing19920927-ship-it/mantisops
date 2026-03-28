package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/model"
	"mantisops/server/internal/store"
)

type GroupHandler struct {
	groupStore  *store.GroupStore
	serverStore *store.ServerStore
	permCache   *PermissionCache // optional, for cache invalidation on group changes
}

func NewGroupHandler(gs *store.GroupStore, ss *store.ServerStore, pc ...*PermissionCache) *GroupHandler {
	h := &GroupHandler{groupStore: gs, serverStore: ss}
	if len(pc) > 0 {
		h.permCache = pc[0]
	}
	return h
}

func (h *GroupHandler) List(c *gin.Context) {
	groups, err := h.groupStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if groups == nil {
		groups = []model.ServerGroup{}
	}
	c.JSON(http.StatusOK, groups)
}

func (h *GroupHandler) Create(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name required"})
		return
	}
	id, err := h.groupStore.Create(body.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": body.Name})
}

func (h *GroupHandler) Update(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var body struct {
		Name      string `json:"name"`
		SortOrder int    `json:"sort_order"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.groupStore.Update(id, body.Name, body.SortOrder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *GroupHandler) Delete(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.groupStore.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.permCache != nil {
		h.permCache.InvalidateAll()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchSortGroups updates sort_order for multiple groups at once.
func (h *GroupHandler) BatchSortGroups(c *gin.Context) {
	var body struct {
		Items []struct {
			ID        int `json:"id"`
			SortOrder int `json:"sort_order"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items := make([]struct{ ID, SortOrder int }, len(body.Items))
	for i, item := range body.Items {
		items[i] = struct{ ID, SortOrder int }{item.ID, item.SortOrder}
	}
	if err := h.groupStore.BatchUpdateSortOrder(items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// BatchSortServers updates sort_order for multiple servers at once.
func (h *GroupHandler) BatchSortServers(c *gin.Context) {
	var body struct {
		Items []struct {
			HostID    string `json:"host_id"`
			SortOrder int    `json:"sort_order"`
		} `json:"items"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	items := make([]struct{ HostID string; SortOrder int }, len(body.Items))
	for i, item := range body.Items {
		items[i] = struct{ HostID string; SortOrder int }{item.HostID, item.SortOrder}
	}
	if err := h.serverStore.BatchUpdateSortOrder(items); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *GroupHandler) SetServerGroup(c *gin.Context) {
	hostID := c.Param("id")
	var body struct {
		GroupID *int `json:"group_id"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.serverStore.SetGroupID(hostID, body.GroupID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.permCache != nil {
		h.permCache.InvalidateAll()
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
