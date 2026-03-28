package api

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/ai"
	"mantisops/server/internal/config"
	"mantisops/server/internal/crypto"
	"mantisops/server/internal/store"
)

type AIHandler struct {
	store     *store.AIStore
	reporter  *ai.Reporter
	chat      *ai.ChatEngine
	provider  *ai.ProviderManager
	scheduler *ai.Scheduler
	settings  *store.SettingsStore
	masterKey []byte
}

func NewAIHandler(
	store *store.AIStore,
	reporter *ai.Reporter,
	chat *ai.ChatEngine,
	provider *ai.ProviderManager,
	scheduler *ai.Scheduler,
	settings *store.SettingsStore,
	masterKey []byte,
) *AIHandler {
	return &AIHandler{
		store:     store,
		reporter:  reporter,
		chat:      chat,
		provider:  provider,
		scheduler: scheduler,
		settings:  settings,
		masterKey: masterKey,
	}
}

// ---------------------------------------------------------------------------
// Report endpoints
// ---------------------------------------------------------------------------

// ListReports returns a paginated list of reports with optional filters.
func (h *AIHandler) ListReports(c *gin.Context) {
	filter := store.ReportFilter{
		Type:   c.Query("type"),
		Status: c.Query("status"),
		Limit:  parseInt(c.Query("limit"), 20),
		Offset: parseInt(c.Query("offset"), 0),
	}
	if filter.Limit > 100 {
		filter.Limit = 100
	}

	reports, total, err := h.store.ListReports(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if reports == nil {
		reports = []store.AIReport{}
	}
	c.JSON(http.StatusOK, gin.H{"reports": reports, "total": total})
}

// GetReport returns a single report including its full content.
func (h *AIHandler) GetReport(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	report, err := h.store.GetReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

// GenerateReport triggers asynchronous report generation.
func (h *AIHandler) GenerateReport(c *gin.Context) {
	if h.reporter == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI 功能未启用，请在 server.yaml 中设置 ai.enabled: true 并重启服务"})
		return
	}
	var req struct {
		ReportType  string `json:"report_type"`
		PeriodStart *int64 `json:"period_start"`
		PeriodEnd   *int64 `json:"period_end"`
		Force       bool   `json:"force"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate report_type
	validTypes := map[string]bool{
		"daily": true, "weekly": true, "monthly": true,
		"quarterly": true, "yearly": true,
	}
	if !validTypes[req.ReportType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "report_type must be one of: daily, weekly, monthly, quarterly, yearly"})
		return
	}

	var periodStart, periodEnd int64
	if req.PeriodStart != nil {
		periodStart = *req.PeriodStart
	}
	if req.PeriodEnd != nil {
		periodEnd = *req.PeriodEnd
	}

	// Launch generation in a goroutine (non-blocking).
	// Use a separate context so it outlives the HTTP request.
	ctx := context.Background()
	reportID := make(chan int64, 1)
	errCh := make(chan error, 1)

	go func() {
		id, err := h.reporter.Generate(ctx, req.ReportType, periodStart, periodEnd, "manual", req.Force)
		reportID <- id
		errCh <- err
	}()

	// Wait briefly to detect immediate errors (conflict, validation).
	select {
	case id := <-reportID:
		err := <-errCh
		if err != nil {
			errMsg := err.Error()
			// Check if it's a conflict (existing report)
			if strings.Contains(errMsg, "report already exists") {
				// Extract existing report ID from error message
				var existingID int64
				fmt.Sscanf(errMsg, "report already exists (id=%d)", &existingID)
				c.JSON(http.StatusConflict, gin.H{
					"error":              errMsg,
					"existing_report_id": existingID,
				})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
			return
		}
		c.JSON(http.StatusOK, gin.H{"report_id": id, "status": "generating"})

	case <-time.After(500 * time.Millisecond):
		// Generation is running in background, return immediately.
		// We don't have the ID yet since Generate creates the record internally.
		c.JSON(http.StatusAccepted, gin.H{"status": "generating"})
	}
}

// DeleteReport removes a report by ID.
func (h *AIHandler) DeleteReport(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.store.DeleteReport(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// DownloadReport returns raw markdown content as a file download.
func (h *AIHandler) DownloadReport(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	report, err := h.store.GetReport(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	filename := fmt.Sprintf("%s.md", report.Title)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.String(http.StatusOK, report.Content)
}

// LatestReport returns the latest completed report (without content).
func (h *AIHandler) LatestReport(c *gin.Context) {
	report, err := h.store.LatestCompletedReport()
	if err == sql.ErrNoRows {
		c.JSON(http.StatusOK, gin.H{})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, report)
}

// ---------------------------------------------------------------------------
// Conversation endpoints
// ---------------------------------------------------------------------------

// ListConversations returns paginated conversations for the current user.
func (h *AIHandler) ListConversations(c *gin.Context) {
	limit := parseInt(c.Query("limit"), 20)
	offset := parseInt(c.Query("offset"), 0)
	if limit > 100 {
		limit = 100
	}

	username, _ := c.Get("username")
	user, _ := username.(string)
	if user == "" {
		user = "admin"
	}

	convs, total, err := h.store.ListConversations(user, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if convs == nil {
		convs = []store.AIConversation{}
	}
	c.JSON(http.StatusOK, gin.H{"conversations": convs, "total": total})
}

// CreateConversation creates a new empty conversation.
func (h *AIHandler) CreateConversation(c *gin.Context) {
	username, _ := c.Get("username")
	user, _ := username.(string)
	if user == "" {
		user = "admin"
	}

	conv := &store.AIConversation{
		Title: "New Chat",
		User:  user,
	}
	id, err := h.store.CreateConversation(conv)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"id": id})
}

// GetConversation returns a conversation with all its messages.
func (h *AIHandler) GetConversation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	conv, err := h.store.GetConversation(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	messages, err := h.store.GetMessagesByConversation(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if messages == nil {
		messages = []store.AIMessage{}
	}

	c.JSON(http.StatusOK, gin.H{"conversation": conv, "messages": messages})
}

// DeleteConversation removes a conversation by ID.
func (h *AIHandler) DeleteConversation(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.store.DeleteConversation(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// SendMessage sends a user message and starts streaming the AI response.
func (h *AIHandler) SendMessage(c *gin.Context) {
	if h.chat == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AI 功能未启用"})
		return
	}
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Content   string `json:"content"`
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.Content) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	username, _ := c.Get("username")
	user, _ := username.(string)
	if user == "" {
		user = "admin"
	}

	userMsgID, assistantMsgID, streamID, err := h.chat.SendMessage(id, req.Content, req.RequestID, user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_message_id":      userMsgID,
		"assistant_message_id": assistantMsgID,
		"stream_id":            streamID,
		"status":               "streaming",
	})
}

// ---------------------------------------------------------------------------
// Settings endpoints
// ---------------------------------------------------------------------------

// GetAISettings returns AI configuration with masked API keys.
func (h *AIHandler) GetAISettings(c *gin.Context) {
	all, err := h.settings.GetAll()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Decrypt and mask API keys
	claudeKey := h.decryptSetting(all, "ai.claude.api_key")
	openaiKey := h.decryptSetting(all, "ai.openai.api_key")

	c.JSON(http.StatusOK, gin.H{
		"active_provider": h.getSetting(all, "ai.active_provider"),
		"claude": gin.H{
			"api_key":      maskAPIKey(claudeKey),
			"report_model": h.getSetting(all, "ai.claude.report_model"),
			"chat_model":   h.getSetting(all, "ai.claude.chat_model"),
		},
		"openai": gin.H{
			"api_key":      maskAPIKey(openaiKey),
			"base_url":     h.getSetting(all, "ai.openai.base_url"),
			"report_model": h.getSetting(all, "ai.openai.report_model"),
			"chat_model":   h.getSetting(all, "ai.openai.chat_model"),
		},
		"ollama": gin.H{
			"host":         h.getSetting(all, "ai.ollama.host"),
			"report_model": h.getSetting(all, "ai.ollama.report_model"),
			"chat_model":   h.getSetting(all, "ai.ollama.chat_model"),
		},
	})
}

// UpdateAISettings updates AI provider configuration.
func (h *AIHandler) UpdateAISettings(c *gin.Context) {
	var req struct {
		ActiveProvider *string `json:"active_provider"`
		Claude         *struct {
			APIKey      *string `json:"api_key"`
			ReportModel *string `json:"report_model"`
			ChatModel   *string `json:"chat_model"`
		} `json:"claude"`
		OpenAI *struct {
			APIKey      *string `json:"api_key"`
			BaseURL     *string `json:"base_url"`
			ReportModel *string `json:"report_model"`
			ChatModel   *string `json:"chat_model"`
		} `json:"openai"`
		Ollama *struct {
			Host        *string `json:"host"`
			ReportModel *string `json:"report_model"`
			ChatModel   *string `json:"chat_model"`
		} `json:"ollama"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Helper to set a plain-text setting
	set := func(key, value string) error {
		return h.settings.Set(key, value)
	}

	// Helper to encrypt and set an API key
	setEncrypted := func(key, value string) error {
		if value == "" {
			return h.settings.Set(key, "")
		}
		encrypted, err := crypto.Encrypt(h.masterKey, []byte(value))
		if err != nil {
			return fmt.Errorf("encrypt %s: %w", key, err)
		}
		return h.settings.Set(key, encrypted)
	}

	if req.ActiveProvider != nil {
		if err := set("ai.active_provider", *req.ActiveProvider); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}

	if req.Claude != nil {
		if req.Claude.APIKey != nil {
			if err := setEncrypted("ai.claude.api_key", *req.Claude.APIKey); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		if req.Claude.ReportModel != nil {
			if err := set("ai.claude.report_model", *req.Claude.ReportModel); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		if req.Claude.ChatModel != nil {
			if err := set("ai.claude.chat_model", *req.Claude.ChatModel); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	if req.OpenAI != nil {
		if req.OpenAI.APIKey != nil {
			if err := setEncrypted("ai.openai.api_key", *req.OpenAI.APIKey); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		if req.OpenAI.BaseURL != nil {
			if err := set("ai.openai.base_url", *req.OpenAI.BaseURL); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		if req.OpenAI.ReportModel != nil {
			if err := set("ai.openai.report_model", *req.OpenAI.ReportModel); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		if req.OpenAI.ChatModel != nil {
			if err := set("ai.openai.chat_model", *req.OpenAI.ChatModel); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	if req.Ollama != nil {
		if req.Ollama.Host != nil {
			if err := set("ai.ollama.host", *req.Ollama.Host); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		if req.Ollama.ReportModel != nil {
			if err := set("ai.ollama.report_model", *req.Ollama.ReportModel); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
		if req.Ollama.ChatModel != nil {
			if err := set("ai.ollama.chat_model", *req.Ollama.ChatModel); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
				return
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ListProviders returns information about all registered AI providers.
func (h *AIHandler) ListProviders(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusOK, gin.H{"providers": []ai.ProviderInfo{}})
		return
	}
	providers := h.provider.List()
	c.JSON(http.StatusOK, gin.H{"providers": providers})
}

// TestProvider tests connectivity to an AI provider with a simple prompt.
func (h *AIHandler) TestProvider(c *gin.Context) {
	var req struct {
		Provider string `json:"provider"`
		APIKey   string `json:"api_key"`
		Host     string `json:"host"`
		Model    string `json:"model"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Provider == "" || req.Model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider and model are required"})
		return
	}

	log.Printf("[ai-test] provider=%s host=%q model=%s", req.Provider, req.Host, req.Model)

	// Create a temporary provider for testing.
	var prov ai.Provider
	switch req.Provider {
	case "claude":
		if req.APIKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "api_key is required for claude"})
			return
		}
		prov = ai.NewClaudeProvider(req.APIKey, config.ClaudeConfig{})
	case "openai":
		if req.APIKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "api_key is required for openai"})
			return
		}
		prov = ai.NewOpenAIProvider(req.APIKey, config.OpenAIConfig{BaseURL: req.Host})
	case "ollama":
		host := req.Host
		if host == "" {
			host = "http://localhost:11434"
		}
		prov = ai.NewOllamaProvider(host, config.OllamaConfig{})
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unknown provider: %s", req.Provider)})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	_, err := prov.Complete(ctx, &ai.CompletionRequest{
		Model: req.Model,
		Messages: []ai.Message{
			{Role: "user", Content: "回复 OK"},
		},
	})
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"ok": false, "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------------------------------------------------------------------------
// Schedule endpoints
// ---------------------------------------------------------------------------

// ListSchedules returns all report generation schedules.
func (h *AIHandler) ListSchedules(c *gin.Context) {
	schedules, err := h.store.ListSchedules()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if schedules == nil {
		schedules = []store.AISchedule{}
	}
	c.JSON(http.StatusOK, gin.H{"schedules": schedules})
}

// UpdateSchedule updates a schedule's enabled status and cron expression.
func (h *AIHandler) UpdateSchedule(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Enabled  *bool   `json:"enabled"`
		CronExpr *string `json:"cron_expr"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get current schedule to merge with partial update.
	existing, err := h.store.GetSchedule(id)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	enabled := existing.Enabled
	cronExpr := existing.CronExpr
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.CronExpr != nil {
		cronExpr = *req.CronExpr
	}

	if err := h.store.UpdateSchedule(id, enabled, cronExpr); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// maskAPIKey masks an API key, showing only the first 8 and last 4 characters.
func maskAPIKey(key string) string {
	if key == "" {
		return ""
	}
	if len(key) <= 12 {
		return "****"
	}
	return key[:8] + "****" + key[len(key)-4:]
}

// getSetting returns a setting value or empty string.
func (h *AIHandler) getSetting(all map[string]string, key string) string {
	if v, ok := all[key]; ok {
		return v
	}
	return ""
}

// decryptSetting retrieves and decrypts an encrypted setting value.
func (h *AIHandler) decryptSetting(all map[string]string, key string) string {
	encrypted, ok := all[key]
	if !ok || encrypted == "" {
		return ""
	}
	if len(h.masterKey) == 0 {
		return ""
	}
	plain, err := crypto.Decrypt(h.masterKey, encrypted)
	if err != nil {
		return ""
	}
	return string(plain)
}
