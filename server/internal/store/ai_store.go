package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// ---------- Model Types ----------

type AIReport struct {
	ID               int64  `json:"id"`
	ReportType       string `json:"report_type"`
	Title            string `json:"title"`
	Summary          string `json:"summary"`
	Content          string `json:"content,omitempty"`
	PeriodStart      int64  `json:"period_start"`
	PeriodEnd        int64  `json:"period_end"`
	Status           string `json:"status"`
	ErrorMessage     string `json:"error_message,omitempty"`
	TriggerType      string `json:"trigger_type"`
	Provider         string `json:"provider"`
	Model            string `json:"model"`
	TokenUsage       int    `json:"token_usage"`
	GenerationTimeMs int64  `json:"generation_time_ms"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type ReportFilter struct {
	Type              string
	Status            string
	IncludeSuperseded bool
	Limit             int
	Offset            int
}

type AIConversation struct {
	ID            int64  `json:"id"`
	Title         string `json:"title"`
	User          string `json:"user"`
	Provider      string `json:"provider"`
	Model         string `json:"model"`
	MessageCount  int    `json:"message_count"`
	LastMessageAt *int64 `json:"last_message_at"`
	CreatedAt     string `json:"created_at"`
}

type AIMessage struct {
	ID               int64  `json:"id"`
	ConversationID   int64  `json:"conversation_id"`
	Role             string `json:"role"`
	Content          string `json:"content"`
	Status           string `json:"status"`
	ErrorMessage     string `json:"error_message,omitempty"`
	RequestID        string `json:"request_id,omitempty"`
	PromptTokens     int    `json:"prompt_tokens,omitempty"`
	CompletionTokens int    `json:"completion_tokens,omitempty"`
	CreatedAt        string `json:"created_at"`
}

type AISchedule struct {
	ID         int64  `json:"id"`
	ReportType string `json:"report_type"`
	Enabled    bool   `json:"enabled"`
	CronExpr   string `json:"cron_expr"`
	LastRunAt  *int64 `json:"last_run_at"`
	NextRunAt  *int64 `json:"next_run_at"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

// ---------- AIStore ----------

type AIStore struct {
	db *sql.DB
}

func NewAIStore(db *sql.DB) *AIStore {
	return &AIStore{db: db}
}

// ---------- Report Methods ----------

func (s *AIStore) CreateReport(r *AIReport) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO ai_reports (report_type, title, summary, content, period_start, period_end, status, error_message, trigger_type, provider, model, token_usage, generation_time_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ReportType, r.Title, r.Summary, r.Content, r.PeriodStart, r.PeriodEnd,
		r.Status, r.ErrorMessage, r.TriggerType, r.Provider, r.Model,
		r.TokenUsage, r.GenerationTimeMs)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *AIStore) GetReport(id int64) (*AIReport, error) {
	var r AIReport
	err := s.db.QueryRow(
		`SELECT id, report_type, title, summary, content, period_start, period_end,
		        status, COALESCE(error_message,''), trigger_type, provider, model,
		        token_usage, generation_time_ms, created_at, updated_at
		FROM ai_reports WHERE id=?`, id).
		Scan(&r.ID, &r.ReportType, &r.Title, &r.Summary, &r.Content,
			&r.PeriodStart, &r.PeriodEnd, &r.Status, &r.ErrorMessage,
			&r.TriggerType, &r.Provider, &r.Model, &r.TokenUsage,
			&r.GenerationTimeMs, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *AIStore) ListReports(f ReportFilter) ([]AIReport, int, error) {
	var where []string
	var args []interface{}

	if f.Type != "" {
		where = append(where, "report_type=?")
		args = append(args, f.Type)
	}
	if f.Status != "" {
		where = append(where, "status=?")
		args = append(args, f.Status)
	}
	if !f.IncludeSuperseded {
		where = append(where, "status!='superseded'")
	}

	whereClause := ""
	if len(where) > 0 {
		whereClause = " WHERE " + strings.Join(where, " AND ")
	}

	// Count total
	var total int
	countQuery := "SELECT COUNT(*) FROM ai_reports" + whereClause
	if err := s.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Default limit
	limit := f.Limit
	if limit == 0 {
		limit = 20
	}

	// List query: exclude Content field (too large)
	listQuery := fmt.Sprintf(
		`SELECT id, report_type, title, summary, period_start, period_end,
		        status, COALESCE(error_message,''), trigger_type, provider, model,
		        token_usage, generation_time_ms, created_at, updated_at
		FROM ai_reports%s ORDER BY created_at DESC LIMIT ? OFFSET ?`, whereClause)

	listArgs := append(args, limit, f.Offset)
	rows, err := s.db.Query(listQuery, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []AIReport
	for rows.Next() {
		var r AIReport
		if err := rows.Scan(&r.ID, &r.ReportType, &r.Title, &r.Summary,
			&r.PeriodStart, &r.PeriodEnd, &r.Status, &r.ErrorMessage,
			&r.TriggerType, &r.Provider, &r.Model, &r.TokenUsage,
			&r.GenerationTimeMs, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, 0, err
		}
		reports = append(reports, r)
	}
	return reports, total, nil
}

func (s *AIStore) UpdateReportStatus(id int64, status, errorMsg string) error {
	_, err := s.db.Exec(
		`UPDATE ai_reports SET status=?, error_message=?, updated_at=? WHERE id=?`,
		status, errorMsg, time.Now(), id)
	return err
}

func (s *AIStore) FindCompletedReport(reportType string, periodStart, periodEnd int64) (*AIReport, error) {
	var r AIReport
	err := s.db.QueryRow(
		`SELECT id, report_type, title, summary, content, period_start, period_end,
		        status, COALESCE(error_message,''), trigger_type, provider, model,
		        token_usage, generation_time_ms, created_at, updated_at
		FROM ai_reports
		WHERE report_type=? AND period_start=? AND period_end=? AND status='completed'`, reportType, periodStart, periodEnd).
		Scan(&r.ID, &r.ReportType, &r.Title, &r.Summary, &r.Content,
			&r.PeriodStart, &r.PeriodEnd, &r.Status, &r.ErrorMessage,
			&r.TriggerType, &r.Provider, &r.Model, &r.TokenUsage,
			&r.GenerationTimeMs, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *AIStore) LatestCompletedReport() (*AIReport, error) {
	var r AIReport
	err := s.db.QueryRow(
		`SELECT id, report_type, title, summary, period_start, period_end,
		        status, COALESCE(error_message,''), trigger_type, provider, model,
		        token_usage, generation_time_ms, created_at, updated_at
		FROM ai_reports WHERE status='completed' ORDER BY created_at DESC LIMIT 1`).
		Scan(&r.ID, &r.ReportType, &r.Title, &r.Summary,
			&r.PeriodStart, &r.PeriodEnd, &r.Status, &r.ErrorMessage,
			&r.TriggerType, &r.Provider, &r.Model, &r.TokenUsage,
			&r.GenerationTimeMs, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *AIStore) DeleteReport(id int64) error {
	_, err := s.db.Exec("DELETE FROM ai_reports WHERE id=?", id)
	return err
}

func (s *AIStore) CleanupStaleReports() error {
	_, err := s.db.Exec(
		`UPDATE ai_reports SET status='failed', error_message='server_restart', updated_at=?
		WHERE status IN ('pending','generating')`, time.Now())
	return err
}

func (s *AIStore) CompleteReportInTx(reportID int64, content, summary, provider, model string, tokenUsage int, genTimeMs int64, supersedeID *int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Supersede old report if needed
	if supersedeID != nil {
		if _, err := tx.Exec(
			`UPDATE ai_reports SET status='superseded', updated_at=? WHERE id=?`,
			time.Now(), *supersedeID); err != nil {
			return err
		}
	}

	// Complete current report
	if _, err := tx.Exec(
		`UPDATE ai_reports SET content=?, summary=?, provider=?, model=?, token_usage=?,
		        generation_time_ms=?, status='completed', updated_at=?
		WHERE id=?`,
		content, summary, provider, model, tokenUsage, genTimeMs, time.Now(), reportID); err != nil {
		return err
	}

	return tx.Commit()
}

// ---------- Conversation Methods ----------

func (s *AIStore) CreateConversation(c *AIConversation) (int64, error) {
	result, err := s.db.Exec(
		`INSERT INTO ai_conversations (title, user, provider, model) VALUES (?, ?, ?, ?)`,
		c.Title, c.User, c.Provider, c.Model)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (s *AIStore) GetConversation(id int64) (*AIConversation, error) {
	var c AIConversation
	err := s.db.QueryRow(
		`SELECT id, title, user, provider, model, message_count, last_message_at, created_at
		FROM ai_conversations WHERE id=?`, id).
		Scan(&c.ID, &c.Title, &c.User, &c.Provider, &c.Model, &c.MessageCount, &c.LastMessageAt, &c.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (s *AIStore) ListConversations(user string, limit, offset int) ([]AIConversation, int, error) {
	// Count total
	var total int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM ai_conversations WHERE user=?`, user).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.Query(
		`SELECT id, title, user, provider, model, message_count, last_message_at, created_at
		FROM ai_conversations WHERE user=? ORDER BY last_message_at DESC LIMIT ? OFFSET ?`,
		user, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var convs []AIConversation
	for rows.Next() {
		var c AIConversation
		if err := rows.Scan(&c.ID, &c.Title, &c.User, &c.Provider, &c.Model,
			&c.MessageCount, &c.LastMessageAt, &c.CreatedAt); err != nil {
			return nil, 0, err
		}
		convs = append(convs, c)
	}
	return convs, total, nil
}

func (s *AIStore) UpdateConversationTitle(id int64, title string) error {
	_, err := s.db.Exec(`UPDATE ai_conversations SET title=? WHERE id=?`, title, id)
	return err
}

func (s *AIStore) DeleteConversation(id int64) error {
	_, err := s.db.Exec("DELETE FROM ai_conversations WHERE id=?", id)
	return err
}

// ---------- Message Methods ----------

func (s *AIStore) CreateMessage(m *AIMessage) (int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Insert message
	result, err := tx.Exec(
		`INSERT INTO ai_messages (conversation_id, role, content, status, error_message, request_id, prompt_tokens, completion_tokens)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ConversationID, m.Role, m.Content, m.Status, m.ErrorMessage,
		m.RequestID, m.PromptTokens, m.CompletionTokens)
	if err != nil {
		return 0, err
	}
	msgID, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Atomically update conversation counters
	now := time.Now().Unix()
	if _, err := tx.Exec(
		`UPDATE ai_conversations SET message_count=message_count+1, last_message_at=? WHERE id=?`,
		now, m.ConversationID); err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return msgID, nil
}

func (s *AIStore) GetMessagesByConversation(convID int64) ([]AIMessage, error) {
	rows, err := s.db.Query(
		`SELECT id, conversation_id, role, content, status, COALESCE(error_message,''),
		        COALESCE(request_id,''), prompt_tokens, completion_tokens, created_at
		FROM ai_messages WHERE conversation_id=? ORDER BY created_at`, convID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []AIMessage
	for rows.Next() {
		var m AIMessage
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content,
			&m.Status, &m.ErrorMessage, &m.RequestID,
			&m.PromptTokens, &m.CompletionTokens, &m.CreatedAt); err != nil {
			return nil, err
		}
		messages = append(messages, m)
	}
	return messages, nil
}

func (s *AIStore) UpdateMessageCompleted(id int64, content string, promptTokens, completionTokens int) error {
	_, err := s.db.Exec(
		`UPDATE ai_messages SET content=?, status='done', prompt_tokens=?, completion_tokens=? WHERE id=?`,
		content, promptTokens, completionTokens, id)
	return err
}

func (s *AIStore) UpdateMessageFailed(id int64, errorMsg string) error {
	_, err := s.db.Exec(
		`UPDATE ai_messages SET status='failed', error_message=? WHERE id=?`,
		errorMsg, id)
	return err
}

func (s *AIStore) FindByRequestID(convID int64, requestID string) (*AIMessage, error) {
	var m AIMessage
	err := s.db.QueryRow(
		`SELECT id, conversation_id, role, content, status, COALESCE(error_message,''),
		        COALESCE(request_id,''), prompt_tokens, completion_tokens, created_at
		FROM ai_messages WHERE conversation_id=? AND request_id=?`, convID, requestID).
		Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Status,
			&m.ErrorMessage, &m.RequestID, &m.PromptTokens, &m.CompletionTokens, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// ---------- Schedule Methods ----------

func (s *AIStore) ListSchedules() ([]AISchedule, error) {
	rows, err := s.db.Query(
		`SELECT id, report_type, enabled, cron_expr, last_run_at, next_run_at, created_at, updated_at
		FROM ai_schedules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []AISchedule
	for rows.Next() {
		var sc AISchedule
		if err := rows.Scan(&sc.ID, &sc.ReportType, &sc.Enabled, &sc.CronExpr,
			&sc.LastRunAt, &sc.NextRunAt, &sc.CreatedAt, &sc.UpdatedAt); err != nil {
			return nil, err
		}
		schedules = append(schedules, sc)
	}
	return schedules, nil
}

func (s *AIStore) GetSchedule(id int64) (*AISchedule, error) {
	var sc AISchedule
	err := s.db.QueryRow(
		`SELECT id, report_type, enabled, cron_expr, last_run_at, next_run_at, created_at, updated_at
		FROM ai_schedules WHERE id=?`, id).
		Scan(&sc.ID, &sc.ReportType, &sc.Enabled, &sc.CronExpr,
			&sc.LastRunAt, &sc.NextRunAt, &sc.CreatedAt, &sc.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &sc, nil
}

func (s *AIStore) UpdateSchedule(id int64, enabled bool, cronExpr string) error {
	_, err := s.db.Exec(
		`UPDATE ai_schedules SET enabled=?, cron_expr=?, updated_at=? WHERE id=?`,
		enabled, cronExpr, time.Now(), id)
	return err
}

func (s *AIStore) UpdateScheduleRun(id int64, lastRunAt, nextRunAt int64) error {
	_, err := s.db.Exec(
		`UPDATE ai_schedules SET last_run_at=?, next_run_at=?, updated_at=? WHERE id=?`,
		lastRunAt, nextRunAt, time.Now(), id)
	return err
}
