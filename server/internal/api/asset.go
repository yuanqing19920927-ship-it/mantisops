package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/model"
	"mantisops/server/internal/store"
)

type AssetHandler struct {
	store           *store.AssetStore
	discoveredStore *store.DiscoveredServiceStore
	// TODO: add permCache and filter assets by server visibility (requires server_id → host_id mapping)
}

func NewAssetHandler(s *store.AssetStore, ds *store.DiscoveredServiceStore) *AssetHandler {
	return &AssetHandler{store: s, discoveredStore: ds}
}

func (h *AssetHandler) List(c *gin.Context) {
	assets, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, assets)
}

func (h *AssetHandler) Create(c *gin.Context) {
	var asset model.Asset
	if err := c.ShouldBindJSON(&asset); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if asset.Status == "" {
		asset.Status = "active"
	}
	id, err := h.store.Create(&asset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	asset.ID = int(id)
	c.JSON(http.StatusCreated, asset)
}

func (h *AssetHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	var asset model.Asset
	if err := c.ShouldBindJSON(&asset); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	asset.ID = id
	if err := h.store.Update(&asset); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, asset)
}

func (h *AssetHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	if err := h.store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AssetHandler) ListDiscovered(c *gin.Context) {
	if h.discoveredStore == nil {
		c.JSON(http.StatusOK, map[string][]store.DiscoveredService{})
		return
	}
	result, err := h.discoveredStore.ListAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if result == nil {
		result = make(map[string][]store.DiscoveredService)
	}
	c.JSON(http.StatusOK, result)
}
