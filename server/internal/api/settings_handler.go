package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/store"
)

type SettingsHandler struct {
	store *store.SettingsStore
}

func NewSettingsHandler(store *store.SettingsStore) *SettingsHandler {
	return &SettingsHandler{store: store}
}

// Get returns all platform settings.
func (h *SettingsHandler) Get(c *gin.Context) {
	all, err := h.store.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"platform_name":     all["platform_name"],
		"platform_subtitle": all["platform_subtitle"],
		"logo_url":          all["logo_url"],
	})
}

// Update saves platform settings.
func (h *SettingsHandler) Update(c *gin.Context) {
	var req struct {
		PlatformName     *string `json:"platform_name"`
		PlatformSubtitle *string `json:"platform_subtitle"`
		LogoURL          *string `json:"logo_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.PlatformName != nil {
		if err := h.store.Set("platform_name", *req.PlatformName); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.PlatformSubtitle != nil {
		if err := h.store.Set("platform_subtitle", *req.PlatformSubtitle); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	if req.LogoURL != nil {
		if err := h.store.Set("logo_url", *req.LogoURL); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}
