package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"opsboard/server/internal/cloud"
	"opsboard/server/internal/store"
)

type CloudHandler struct {
	manager   *cloud.Manager
	cloud     *store.CloudStore
	credStore *store.CredentialStore
}

func NewCloudHandler(mgr *cloud.Manager, cs *store.CloudStore, cred *store.CredentialStore) *CloudHandler {
	return &CloudHandler{manager: mgr, cloud: cs, credStore: cred}
}

// ListAccounts returns all cloud accounts.
func (h *CloudHandler) ListAccounts(c *gin.Context) {
	accounts, err := h.cloud.ListAccounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if accounts == nil {
		accounts = []store.CloudAccount{}
	}
	c.JSON(http.StatusOK, accounts)
}

// CreateAccount creates a cloud account, optionally creating an embedded credential first.
func (h *CloudHandler) CreateAccount(c *gin.Context) {
	var req struct {
		Name         string   `json:"name" binding:"required"`
		Provider     string   `json:"provider" binding:"required"`
		CredentialID int      `json:"credential_id"`
		Credential   *struct {
			Name string            `json:"name" binding:"required"`
			Type string            `json:"type" binding:"required"`
			Data map[string]string `json:"data" binding:"required"`
		} `json:"credential"`
		AutoDiscover bool     `json:"auto_discover"`
		RegionIDs    []string `json:"region_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	credID := req.CredentialID

	// If an embedded credential is provided, create it first.
	if req.Credential != nil {
		id, err := h.credStore.Create(req.Credential.Name, req.Credential.Type, req.Credential.Data)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create credential: " + err.Error()})
			return
		}
		credID = id
	}

	if credID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "credential_id or embedded credential required"})
		return
	}

	if req.RegionIDs == nil {
		req.RegionIDs = []string{}
	}

	accountID, err := h.cloud.CreateAccount(req.Name, req.Provider, credID, req.RegionIDs, req.AutoDiscover)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"id": accountID, "credential_id": credID})
}

// UpdateAccount updates a cloud account's name, regions, and auto_discover flag.
func (h *CloudHandler) UpdateAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Name         string   `json:"name" binding:"required"`
		RegionIDs    []string `json:"region_ids"`
		AutoDiscover bool     `json:"auto_discover"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.RegionIDs == nil {
		req.RegionIDs = []string{}
	}
	if err := h.cloud.UpdateAccount(id, req.Name, req.RegionIDs, req.AutoDiscover); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DeleteAccount performs a two-phase delete. Without ?force=true it returns an impact summary.
func (h *CloudHandler) DeleteAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	force := c.Query("force") == "true"

	impact, err := h.manager.DeleteAccount(id, force)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !force {
		c.JSON(http.StatusOK, gin.H{"impact": impact})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Verify validates AK/SK without creating any DB record.
func (h *CloudHandler) Verify(c *gin.Context) {
	var req struct {
		AccessKeyID     string `json:"access_key_id" binding:"required"`
		AccessKeySecret string `json:"access_key_secret" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.manager.Verify(req.AccessKeyID, req.AccessKeySecret)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// SyncAccount triggers async instance discovery for a cloud account.
func (h *CloudHandler) SyncAccount(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.manager.Sync(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "message": "同步已启动"})
}

// ListInstances returns all instances for a given cloud account.
func (h *CloudHandler) ListInstances(c *gin.Context) {
	accountID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	instances, err := h.cloud.ListInstances(accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if instances == nil {
		instances = []store.CloudInstance{}
	}
	c.JSON(http.StatusOK, instances)
}

// ConfirmInstances batch-confirms instances (sets monitored=1 and registers ECS as servers).
func (h *CloudHandler) ConfirmInstances(c *gin.Context) {
	var req struct {
		InstanceIDs []int `json:"instance_ids" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.manager.ConfirmInstances(req.InstanceIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "count": len(req.InstanceIDs)})
}

// UpdateInstance updates a single instance (e.g. enable/disable monitoring).
func (h *CloudHandler) UpdateInstance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var req struct {
		Monitored *bool `json:"monitored"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Monitored != nil {
		if err := h.cloud.UpdateInstanceMonitored(id, *req.Monitored); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// AddInstance manually adds a cloud instance.
func (h *CloudHandler) AddInstance(c *gin.Context) {
	var req struct {
		CloudAccountID int    `json:"cloud_account_id" binding:"required"`
		InstanceType   string `json:"instance_type" binding:"required"`
		InstanceID     string `json:"instance_id" binding:"required"`
		HostID         string `json:"host_id"`
		InstanceName   string `json:"instance_name"`
		RegionID       string `json:"region_id"`
		Spec           string `json:"spec"`
		Engine         string `json:"engine"`
		Endpoint       string `json:"endpoint"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	hostID := req.HostID
	if hostID == "" {
		hostID = req.InstanceType + "-" + req.InstanceID
	}
	inst := &store.CloudInstance{
		CloudAccountID: req.CloudAccountID,
		InstanceType:   req.InstanceType,
		InstanceID:     req.InstanceID,
		HostID:         hostID,
		InstanceName:   req.InstanceName,
		RegionID:       req.RegionID,
		Spec:           req.Spec,
		Engine:         req.Engine,
		Endpoint:       req.Endpoint,
	}
	if err := h.cloud.UpsertInstance(req.CloudAccountID, inst); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

// DeleteInstance performs a two-phase delete for a single instance.
func (h *CloudHandler) DeleteInstance(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	force := c.Query("force") == "true"

	impact, err := h.manager.DeleteInstance(id, force)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !force {
		c.JSON(http.StatusOK, gin.H{"impact": impact})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
