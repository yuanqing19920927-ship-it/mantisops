package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"opsboard/server/internal/deployer"
	"opsboard/server/internal/store"
)

type ManagedServerHandler struct {
	store     *store.ManagedServerStore
	deployer  *deployer.Deployer
	credStore *store.CredentialStore
}

func NewManagedServerHandler(s *store.ManagedServerStore, d *deployer.Deployer, cs *store.CredentialStore) *ManagedServerHandler {
	return &ManagedServerHandler{store: s, deployer: d, credStore: cs}
}

// List returns all managed servers
func (h *ManagedServerHandler) List(c *gin.Context) {
	list, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []store.ManagedServer{}
	}
	c.JSON(http.StatusOK, list)
}

// Create adds a new managed server (with optional embedded credential)
func (h *ManagedServerHandler) Create(c *gin.Context) {
	var req struct {
		Host     string `json:"host" binding:"required"`
		SSHPort  int    `json:"ssh_port"`
		SSHUser  string `json:"ssh_user" binding:"required"`
		HostKey  string `json:"host_key"`
		// Either credential_id or embedded credential
		CredentialID int `json:"credential_id"`
		Credential   *struct {
			Name string            `json:"name"`
			Type string            `json:"type"`
			Data map[string]string `json:"data"`
		} `json:"credential"`
		Options json.RawMessage `json:"options"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}

	credID := req.CredentialID
	if req.Credential != nil && credID == 0 {
		var err error
		credID, err = h.credStore.Create(req.Credential.Name, req.Credential.Type, req.Credential.Data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create credential: " + err.Error()})
			return
		}
	}
	if credID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential_id or credential required"})
		return
	}

	optionsStr := "{}"
	if req.Options != nil {
		optionsStr = string(req.Options)
	}

	ms := &store.ManagedServer{
		Host:           req.Host,
		SSHPort:        req.SSHPort,
		SSHUser:        req.SSHUser,
		CredentialID:   credID,
		SSHHostKey:     req.HostKey,
		InstallOptions: optionsStr,
	}
	id, err := h.store.Create(ms)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ms.ID = id
	c.JSON(http.StatusCreated, ms)
}

// TestConnection performs a dry-run SSH test without saving to DB
func (h *ManagedServerHandler) TestConnection(c *gin.Context) {
	var req deployer.TestConnRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}
	result, err := h.deployer.TestConnection(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// Deploy triggers agent installation
func (h *ManagedServerHandler) Deploy(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.deployer.Deploy(id); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "deployment started"})
}

// Retry retries a failed deployment (same as Deploy - CAS handles the state check)
func (h *ManagedServerHandler) Retry(c *gin.Context) {
	h.Deploy(c) // CAS in Deploy() only allows pending/failed states
}

// Delete removes a managed server record
func (h *ManagedServerHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.store.Delete(id, h.credStore); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Uninstall remotely uninstalls the agent
func (h *ManagedServerHandler) Uninstall(c *gin.Context) {
	// TODO: implement remote uninstall via SSH
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet"})
}
