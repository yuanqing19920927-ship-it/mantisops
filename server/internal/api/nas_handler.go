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
		CredentialID    int    `json:"credential_id"` // 可选：已有凭据
		Password        string `json:"password"`      // 可选：直接输入密码（自动创建凭据）
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

	// 凭据处理：已有 credential_id 或自动创建
	if req.CredentialID > 0 {
		cred, err := h.credStore.Get(req.CredentialID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "credential not found: " + err.Error()})
			return
		}
		if cred.Type != "ssh_password" && cred.Type != "ssh_key" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "credential must be ssh_password or ssh_key type"})
			return
		}
	} else if req.Password != "" {
		// 自动创建 SSH 密码凭据
		credName := fmt.Sprintf("NAS-%s@%s", req.SSHUser, req.Host)
		credID, err := h.credStore.Create(credName, "ssh_password", map[string]string{"password": req.Password})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create credential: " + err.Error()})
			return
		}
		req.CredentialID = credID
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "either credential_id or password is required"})
		return
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
		CredentialID    int    `json:"credential_id"`
		Password        string `json:"password"`
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

	// 凭据处理：已有 credential_id 或通过密码创建/更新
	if req.CredentialID > 0 {
		cred, err := h.credStore.Get(req.CredentialID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "credential not found: " + err.Error()})
			return
		}
		if cred.Type != "ssh_password" && cred.Type != "ssh_key" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "credential must be ssh_password or ssh_key type"})
			return
		}
	} else if req.Password != "" {
		credName := fmt.Sprintf("NAS-%s@%s", req.SSHUser, req.Host)
		credID, err := h.credStore.Create(credName, "ssh_password", map[string]string{"password": req.Password})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "create credential: " + err.Error()})
			return
		}
		req.CredentialID = credID
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "either credential_id or password is required"})
		return
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
		CredentialID int    `json:"credential_id"` // 可选：已有凭据
		Password     string `json:"password"`      // 可选：直接输入密码
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

	// 两种认证方式：credential_id 或直接密码
	var credType, password, privateKey, passphrase string
	if req.CredentialID > 0 {
		cred, err := h.credStore.Get(req.CredentialID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "credential not found: " + err.Error()})
			return
		}
		credType = cred.Type
		password = cred.Data["password"]
		privateKey = cred.Data["private_key"]
		passphrase = cred.Data["passphrase"]
	} else if req.Password != "" {
		credType = "ssh_password"
		password = req.Password
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "either credential_id or password is required"})
		return
	}

	client, err := collector.SSHConnect(
		req.Host, req.Port, req.SSHUser,
		credType, password, privateKey, passphrase,
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

	// Check for fnOS — fnOS 基于 Debian，/etc/os-release 显示 Debian，需要检查 fnOS 特有标识
	if detectedType == "" {
		// 优先检查 fnOS 特有路径
		for _, probe := range []string{
			"test -d /usr/lib/fnos && echo fnos",
			"test -f /etc/fnos-release && echo fnos",
			"dpkg -l 2>/dev/null | grep -q fnos && echo fnos",
			"test -d /opt/apps/fnCloud && echo fnos",
			"cat /etc/issue 2>/dev/null | grep -iq fn && echo fnos",
		} {
			probeOut, probeErr := collector.SSHExec(client, probe, timeout)
			if probeErr == nil && strings.Contains(strings.ToLower(strings.TrimSpace(probeOut)), "fnos") {
				detectedType = "fnos"
				break
			}
		}
	}

	// Check smartctl availability
	smartAvailable := false
	smartOut, smartErr := collector.SSHExec(client, "which smartctl 2>/dev/null || command -v smartctl 2>/dev/null", timeout)
	if smartErr == nil && strings.TrimSpace(smartOut) != "" {
		smartAvailable = true
	}

	// 返回额外诊断信息帮助调试
	debugInfo := map[string]string{}
	if osRelease, osErr := collector.SSHExec(client, "cat /etc/os-release 2>/dev/null | head -5", timeout); osErr == nil {
		debugInfo["os_release"] = strings.TrimSpace(osRelease)
	}
	if issue, issueErr := collector.SSHExec(client, "cat /etc/issue 2>/dev/null", timeout); issueErr == nil {
		debugInfo["issue"] = strings.TrimSpace(issue)
	}
	if hostname, hostErr := collector.SSHExec(client, "hostname 2>/dev/null", timeout); hostErr == nil {
		debugInfo["hostname"] = strings.TrimSpace(hostname)
	}

	c.JSON(http.StatusOK, gin.H{
		"ok":              true,
		"detected_type":   detectedType,
		"smart_available": smartAvailable,
		"debug":           debugInfo,
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
