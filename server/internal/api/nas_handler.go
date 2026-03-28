package api

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/collector"
	"mantisops/server/internal/store"
)

type NasHandler struct {
	nasStore     *store.NasStore
	credStore    *store.CredentialStore
	nasCollector *collector.NasCollector
}

func NewNasHandler(nasStore *store.NasStore, credStore *store.CredentialStore, nasCollector *collector.NasCollector) *NasHandler {
	return &NasHandler{nasStore: nasStore, credStore: credStore, nasCollector: nasCollector}
}

func (h *NasHandler) List(c *gin.Context) {
	devices, err := h.nasStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if devices == nil {
		devices = []store.NasDevice{}
	}
	c.JSON(http.StatusOK, devices)
}

func (h *NasHandler) Create(c *gin.Context) {
	var req struct {
		Name            string `json:"name" binding:"required"`
		NasType         string `json:"nas_type" binding:"required"`
		Host            string `json:"host" binding:"required"`
		Port            int    `json:"port"`
		SSHUser         string `json:"ssh_user"`
		CredentialID    int    `json:"credential_id" binding:"required"`
		CollectInterval int    `json:"collect_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate nas_type
	if req.NasType != "synology" && req.NasType != "fnos" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nas_type must be 'synology' or 'fnos'"})
		return
	}

	// Validate credential type
	cred, err := h.credStore.Get(req.CredentialID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential not found: " + err.Error()})
		return
	}
	if cred.Type != "ssh_password" && cred.Type != "ssh_key" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential must be ssh_password or ssh_key type"})
		return
	}

	// Defaults
	if req.Port == 0 {
		req.Port = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}
	if req.CollectInterval == 0 {
		req.CollectInterval = 60
	}

	id, err := h.nasStore.Create(req.Name, req.NasType, req.Host, req.Port, req.SSHUser, req.CredentialID, req.CollectInterval)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify collector
	device, err := h.nasStore.Get(id)
	if err == nil {
		h.nasCollector.AddDevice(*device)
	}

	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *NasHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Name            string `json:"name" binding:"required"`
		NasType         string `json:"nas_type" binding:"required"`
		Host            string `json:"host" binding:"required"`
		Port            int    `json:"port"`
		SSHUser         string `json:"ssh_user"`
		CredentialID    int    `json:"credential_id" binding:"required"`
		CollectInterval int    `json:"collect_interval"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Port == 0 {
		req.Port = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}
	if req.CollectInterval == 0 {
		req.CollectInterval = 60
	}

	if err := h.nasStore.Update(id, req.Name, req.NasType, req.Host, req.Port, req.SSHUser, req.CredentialID, req.CollectInterval); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Notify collector
	device, err := h.nasStore.Get(id)
	if err == nil {
		h.nasCollector.UpdateDevice(*device)
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NasHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	h.nasCollector.RemoveDevice(id)

	if err := h.nasStore.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NasHandler) TestConnection(c *gin.Context) {
	var req struct {
		Host         string `json:"host" binding:"required"`
		Port         int    `json:"port"`
		SSHUser      string `json:"ssh_user"`
		CredentialID int    `json:"credential_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.Port == 0 {
		req.Port = 22
	}
	if req.SSHUser == "" {
		req.SSHUser = "root"
	}

	cred, err := h.credStore.Get(req.CredentialID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential not found: " + err.Error()})
		return
	}

	client, err := collector.SSHConnect(
		req.Host, req.Port, req.SSHUser,
		cred.Type,
		cred.Data["password"],
		cred.Data["private_key"],
		cred.Data["passphrase"],
	)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": fmt.Sprintf("SSH connection failed: %v", err)})
		return
	}
	defer client.Close()

	// Auto-detect NAS type
	detectedType := ""
	timeout := 10 * time.Second

	// Check for Synology
	out, err := collector.SSHExec(client, "cat /etc/synoinfo.conf 2>/dev/null", timeout)
	if err == nil && strings.TrimSpace(out) != "" {
		detectedType = "synology"
	}

	// Check for fnOS
	if detectedType == "" {
		out, err = collector.SSHExec(client, "cat /etc/os-release 2>/dev/null", timeout)
		if err == nil && strings.Contains(out, "fnos") {
			detectedType = "fnos"
		}
	}

	// Check smartctl availability
	smartAvailable := false
	_, err = collector.SSHExec(client, "which smartctl 2>/dev/null || command -v smartctl 2>/dev/null", timeout)
	if err == nil {
		smartAvailable = true
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"detected_type":   detectedType,
		"smart_available": smartAvailable,
	})
}

func (h *NasHandler) GetMetrics(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	metrics := h.nasCollector.GetMetrics(int64(id))
	if metrics == nil {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	c.JSON(http.StatusOK, metrics)
}
