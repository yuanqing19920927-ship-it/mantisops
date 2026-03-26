package api

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"opsboard/server/internal/model"
	"opsboard/server/internal/probe"
	"opsboard/server/internal/store"
)

type ProbeHandler struct {
	store  *store.ProbeStore
	prober *probe.Prober
}

func NewProbeHandler(s *store.ProbeStore, p *probe.Prober) *ProbeHandler {
	return &ProbeHandler{store: s, prober: p}
}

func (h *ProbeHandler) List(c *gin.Context) {
	rules, err := h.store.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
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
	c.JSON(http.StatusOK, results)
}
