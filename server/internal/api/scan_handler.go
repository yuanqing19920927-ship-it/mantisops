package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/probe"
	"mantisops/server/internal/store"
)

type ScanHandler struct {
	templateStore *store.ScanTemplateStore
	serverStore   *store.ServerStore
	scanner       *probe.Scanner
}

func NewScanHandler(ts *store.ScanTemplateStore, ss *store.ServerStore, scanner *probe.Scanner) *ScanHandler {
	return &ScanHandler{templateStore: ts, serverStore: ss, scanner: scanner}
}

// --- Template CRUD ---

func (h *ScanHandler) ListTemplates(c *gin.Context) {
	list, err := h.templateStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if list == nil {
		list = []store.ScanTemplate{}
	}
	c.JSON(http.StatusOK, list)
}

func (h *ScanHandler) CreateTemplate(c *gin.Context) {
	var body struct {
		Port int    `json:"port"`
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Port <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "port required"})
		return
	}
	id, err := h.templateStore.Create(body.Port, body.Name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (h *ScanHandler) UpdateTemplate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var body struct {
		Port    int    `json:"port"`
		Name    string `json:"name"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	if err := h.templateStore.Update(id, body.Port, body.Name, enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ScanHandler) DeleteTemplate(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.templateStore.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// --- Scan ---

func (h *ScanHandler) StartScan(c *gin.Context) {
	var body struct {
		HostIDs []string `json:"host_ids"`
	}
	c.ShouldBindJSON(&body)

	servers, err := h.serverStore.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	sourceMap := loadSourceMapFromDB(h.serverStore.DB())

	scanAll := len(body.HostIDs) == 0
	hostSet := make(map[string]bool)
	if !scanAll {
		for _, id := range body.HostIDs {
			if id == "all" {
				scanAll = true
				break
			}
			hostSet[id] = true
		}
	}

	var targets []probe.ScanTarget
	for _, s := range servers {
		if !scanAll && !hostSet[s.HostID] {
			continue
		}
		ip := ""
		var ips []string
		json.Unmarshal([]byte(s.IPAddresses), &ips)
		if len(ips) > 0 {
			ip = ips[0]
		}
		if ip == "" {
			continue
		}
		source := "agent"
		if src, ok := sourceMap[s.HostID]; ok {
			source = src
		}
		name := s.DisplayName
		if name == "" {
			name = s.Hostname
		}
		targets = append(targets, probe.ScanTarget{
			HostID:     s.HostID,
			ServerID:   s.ID,
			ServerName: name,
			IP:         ip,
			Source:     source,
		})
	}

	if len(targets) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid targets"})
		return
	}

	taskID := h.scanner.Scan(targets)
	c.JSON(http.StatusOK, gin.H{"ok": true, "task_id": taskID})
}

// loadSourceMapFromDB is a standalone helper (same logic as ServerHandler.loadSourceMap)
func loadSourceMapFromDB(db *sql.DB) map[string]string {
	result := make(map[string]string)
	rows, err := db.Query("SELECT agent_host_id FROM managed_servers WHERE agent_host_id != ''")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var hostID string
			if rows.Scan(&hostID) == nil {
				result[hostID] = "managed"
			}
		}
	}
	rows2, err := db.Query("SELECT host_id FROM cloud_instances WHERE instance_type = 'ecs'")
	if err == nil {
		defer rows2.Close()
		for rows2.Next() {
			var hostID string
			if rows2.Scan(&hostID) == nil {
				if _, exists := result[hostID]; !exists {
					result[hostID] = "cloud"
				}
			}
		}
	}
	return result
}
