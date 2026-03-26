package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"opsboard/server/internal/alert"
	"opsboard/server/internal/model"
	"opsboard/server/internal/store"
)

type AlertHandler struct {
	store   *store.AlertStore
	alerter *alert.Alerter
}

func NewAlertHandler(s *store.AlertStore, a *alert.Alerter) *AlertHandler {
	return &AlertHandler{store: s, alerter: a}
}

// ---------- Rules CRUD ----------

func (h *AlertHandler) ListRules(c *gin.Context) {
	rules, err := h.store.ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rules)
}

func (h *AlertHandler) CreateRule(c *gin.Context) {
	var rule model.AlertRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Set defaults
	if rule.Operator == "" {
		rule.Operator = ">"
	}
	if rule.Duration == 0 {
		rule.Duration = 3
	}
	if rule.Level == "" {
		rule.Level = "warning"
	}
	rule.Enabled = true

	id, err := h.store.CreateRule(&rule)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rule.ID = int(id)
	c.JSON(http.StatusOK, rule)
}

func (h *AlertHandler) UpdateRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	// Get current rule state to detect enabled→disabled transition
	currentRules, err := h.store.ListRules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var wasEnabled bool
	for _, r := range currentRules {
		if r.ID == id {
			wasEnabled = r.Enabled
			break
		}
	}

	var rule model.AlertRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	rule.ID = id

	if err := h.store.UpdateRule(&rule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Handle state transitions
	if wasEnabled && !rule.Enabled {
		h.alerter.OnRuleChanged(id, "rule_disabled")
	} else if rule.Enabled {
		h.alerter.OnRuleUpdated(id)
	}

	c.JSON(http.StatusOK, rule)
}

func (h *AlertHandler) DeleteRule(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	h.alerter.OnRuleChanged(id, "rule_deleted")

	if err := h.store.DeleteRule(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// ---------- Events ----------

func (h *AlertHandler) ListEvents(c *gin.Context) {
	status := c.Query("status")

	var silenced *bool
	if s := c.Query("silenced"); s != "" {
		v := s == "true" || s == "1"
		silenced = &v
	}

	since := c.Query("since")
	until := c.Query("until")

	limit := 50
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	events, err := h.store.QueryEvents(status, silenced, since, until, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, events)
}

func (h *AlertHandler) GetStats(c *gin.Context) {
	stats, err := h.store.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *AlertHandler) AckEvent(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	username, _ := c.Get("username")
	usernameStr, _ := username.(string)

	if err := h.alerter.AckEvent(id, usernameStr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AlertHandler) GetEventNotifications(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	notifications, err := h.store.GetEventNotifications(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, notifications)
}

// ---------- Channels CRUD ----------

func (h *AlertHandler) ListChannels(c *gin.Context) {
	channels, err := h.store.ListChannels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, channels)
}

func (h *AlertHandler) CreateChannel(c *gin.Context) {
	var ch model.NotificationChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ch.Enabled = true

	id, err := h.store.CreateChannel(&ch)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ch.ID = int(id)
	c.JSON(http.StatusOK, ch)
}

func (h *AlertHandler) UpdateChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var ch model.NotificationChannel
	if err := c.ShouldBindJSON(&ch); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ch.ID = id

	if err := h.store.UpdateChannel(&ch); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ch)
}

func (h *AlertHandler) DeleteChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.store.DeleteChannel(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *AlertHandler) TestChannel(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	ch, err := h.store.GetChannel(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "channel not found"})
		return
	}

	testEvent := &model.AlertEvent{
		RuleName:    "测试告警",
		TargetLabel: "OpsBoard 测试",
		Level:       "info",
		Message:     "这是一条测试通知，确认渠道配置正确。",
		FiredAt:     time.Now(),
	}

	if err := alert.SendNotification(ch, testEvent, "firing"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "success"})
}
