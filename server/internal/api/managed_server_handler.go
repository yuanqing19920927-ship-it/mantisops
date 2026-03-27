package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/deployer"
	"mantisops/server/internal/store"
)

type ManagedServerHandler struct {
	store     *store.ManagedServerStore
	deployer  *deployer.Deployer
	credStore *store.CredentialStore
}

func NewManagedServerHandler(s *store.ManagedServerStore, d *deployer.Deployer, cs *store.CredentialStore) *ManagedServerHandler {
	return &ManagedServerHandler{store: s, deployer: d, credStore: cs}
}

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

func (h *ManagedServerHandler) Create(c *gin.Context) {
	var req struct {
		Host     string `json:"host" binding:"required"`
		SSHPort  int    `json:"ssh_port"`
		SSHUser  string `json:"ssh_user" binding:"required"`
		HostKey  string `json:"host_key"`
		CredentialID int `json:"credential_id"`
		Credential   *struct {
			Name string            `json:"name"`
			Type string            `json:"type"`
			Data map[string]string `json:"data"`
		} `json:"credential"`
		Options json.RawMessage `json:"options"`
		// Flat credential fields from frontend
		AuthType   string `json:"auth_type"`
		Password   string `json:"password"`
		PrivateKey string `json:"private_key"`
		Passphrase string `json:"passphrase"`
		// Flat option fields from frontend
		AgentID         string `json:"agent_id"`
		CollectInterval int    `json:"collect_interval"`
		EnableDocker    *bool  `json:"enable_docker"`
		EnableGPU       *bool  `json:"enable_gpu"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.SSHPort == 0 {
		req.SSHPort = 22
	}

	// Build credential from flat fields if no structured credential provided
	if req.Credential == nil && req.CredentialID == 0 && req.AuthType != "" {
		credData := map[string]string{}
		credType := req.AuthType
		if credType == "password" {
			credType = "ssh_password"
			credData["password"] = req.Password
		} else if credType == "private_key" || credType == "key" {
			credType = "ssh_key"
			credData["private_key"] = req.PrivateKey
			if req.Passphrase != "" {
				credData["passphrase"] = req.Passphrase
			}
		}
		req.Credential = &struct {
			Name string            `json:"name"`
			Type string            `json:"type"`
			Data map[string]string `json:"data"`
		}{
			Name: req.SSHUser + "@" + req.Host,
			Type: credType,
			Data: credData,
		}
	}

	// Build options from flat fields if no structured options provided
	if req.Options == nil && (req.AgentID != "" || req.CollectInterval != 0 || req.EnableDocker != nil || req.EnableGPU != nil) {
		opts := map[string]interface{}{}
		if req.AgentID != "" {
			opts["agent_id"] = req.AgentID
		}
		if req.CollectInterval != 0 {
			opts["collect_interval"] = req.CollectInterval
		}
		if req.EnableDocker != nil {
			opts["enable_docker"] = *req.EnableDocker
		}
		if req.EnableGPU != nil {
			opts["enable_gpu"] = *req.EnableGPU
		}
		if b, err := json.Marshal(opts); err == nil {
			req.Options = b
		}
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

func (h *ManagedServerHandler) Retry(c *gin.Context) {
	h.Deploy(c)
}

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

func (h *ManagedServerHandler) Uninstall(c *gin.Context) {
	// TODO: implement remote uninstall via SSH
	c.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented yet"})
}
