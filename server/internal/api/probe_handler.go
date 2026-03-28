package api

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/model"
	"mantisops/server/internal/probe"
	"mantisops/server/internal/store"
)

func deriveHostPort(rule *model.ProbeRule) {
	if rule.Protocol != "http" && rule.Protocol != "https" {
		return
	}
	if rule.URL == "" {
		return
	}
	u, err := url.Parse(rule.URL)
	if err != nil {
		return
	}
	rule.Host = u.Hostname()
	port := u.Port()
	if port == "" {
		if rule.Protocol == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	fmt.Sscanf(port, "%d", &rule.Port)
}

type ProbeHandler struct {
	store     *store.ProbeStore
	prober    *probe.Prober
	permCache *PermissionCache
}

func NewProbeHandler(s *store.ProbeStore, p *probe.Prober, pc *PermissionCache) *ProbeHandler {
	return &ProbeHandler{store: s, prober: p, permCache: pc}
}

func (h *ProbeHandler) List(c *gin.Context) {
	rules, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ps := GetPermissionSet(c, h.permCache); ps != nil {
		filtered := rules[:0]
		for _, p := range rules {
			if ps.CanSeeProbe(strconv.Itoa(p.ID)) {
				filtered = append(filtered, p)
			}
		}
		rules = filtered
	}
	c.JSON(http.StatusOK, rules)
}

func (h *ProbeHandler) Create(c *gin.Context) {
	var rule model.ProbeRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if rule.Protocol == "" {
		rule.Protocol = "tcp"
	}
	if rule.IntervalSec == 0 {
		rule.IntervalSec = 30
	}
	if rule.TimeoutSec == 0 {
		rule.TimeoutSec = 5
	}
	rule.Enabled = true
	deriveHostPort(&rule)
	id, err := h.store.Create(&rule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rule.ID = int(id)
	c.JSON(http.StatusCreated, rule)
}

func (h *ProbeHandler) Update(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	var rule model.ProbeRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rule.ID = id
	deriveHostPort(&rule)
	if err := h.store.Update(&rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rule)
}

func (h *ProbeHandler) Delete(c *gin.Context) {
	idStr := c.Param("id")
	id, _ := strconv.Atoi(idStr)
	if err := h.store.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ProbeHandler) Status(c *gin.Context) {
	results := h.prober.GetAllResults()
	if ps := GetPermissionSet(c, h.permCache); ps != nil {
		filtered := results[:0]
		for _, r := range results {
			if ps.CanSeeProbe(strconv.Itoa(r.RuleID)) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
	}
	c.JSON(http.StatusOK, results)
}
