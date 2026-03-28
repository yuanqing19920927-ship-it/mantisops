package ai

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"mantisops/server/internal/config"
	"mantisops/server/internal/store"
	"mantisops/server/internal/ws"
)

// Reporter orchestrates report generation: data collection → prompt assembly →
// LLM call → store result. It serialises generation so that at most one report
// is produced at a time.
type Reporter struct {
	store     *store.AIStore
	collector *DataCollector
	provider  *ProviderManager
	hub       *ws.Hub
	cfg       config.AIConfig
	timezone  *time.Location
	maxTime   time.Duration
	mu        sync.Mutex
}

// NewReporter creates a Reporter from the given dependencies and config.
func NewReporter(aiStore *store.AIStore, collector *DataCollector, provider *ProviderManager, hub *ws.Hub, cfg config.AIConfig) *Reporter {
	tz, err := time.LoadLocation(cfg.Timezone)
	if err != nil {
		tz, _ = time.LoadLocation("Asia/Shanghai")
	}
	maxTime := time.Duration(cfg.Report.MaxGenerationTime) * time.Second
	if maxTime <= 0 {
		maxTime = 300 * time.Second
	}
	return &Reporter{
		store:     aiStore,
		collector: collector,
		provider:  provider,
		hub:       hub,
		cfg:       cfg,
		timezone:  tz,
		maxTime:   maxTime,
	}
}

// Generate runs the full report generation flow: collect data, call LLM, store
// the result and broadcast progress via WebSocket.
func (r *Reporter) Generate(ctx context.Context, reportType string, periodStart, periodEnd int64, triggerType string, force bool) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Default period when not supplied.
	if periodStart == 0 {
		periodStart, periodEnd = CalcPeriod(reportType, r.timezone)
	}

	// Check for an existing completed report for the same period.
	var supersedeID *int64
	existing, err := r.store.FindCompletedReport(reportType, periodStart, periodEnd)
	if err != nil && err != sql.ErrNoRows {
		return 0, fmt.Errorf("check existing report: %w", err)
	}
	if existing != nil {
		if !force {
			return 0, fmt.Errorf("report already exists (id=%d), use force to regenerate", existing.ID)
		}
		supersedeID = &existing.ID
	}

	title := GenerateTitle(reportType, periodStart, r.timezone)

	// Create the report record as pending.
	reportID, err := r.store.CreateReport(&store.AIReport{
		ReportType:  reportType,
		Title:       title,
		PeriodStart: periodStart,
		PeriodEnd:   periodEnd,
		Status:      "pending",
		TriggerType: triggerType,
	})
	if err != nil {
		return 0, fmt.Errorf("create report record: %w", err)
	}

	// Mark as generating.
	if err := r.store.UpdateReportStatus(reportID, "generating", ""); err != nil {
		return reportID, fmt.Errorf("update status to generating: %w", err)
	}

	r.broadcast(map[string]interface{}{
		"type":        "ai_report_generating",
		"report_id":   reportID,
		"report_type": reportType,
		"title":       title,
	})

	genStart := time.Now()

	// Collect data.
	start := time.Unix(periodStart, 0)
	end := time.Unix(periodEnd, 0)
	data, err := r.collector.Collect(reportType, start, end)
	if err != nil {
		r.failReport(reportID, reportType, title, fmt.Sprintf("data collection failed: %v", err))
		return reportID, fmt.Errorf("collect data: %w", err)
	}

	// Format data and build prompt.
	dataText := r.collector.FormatAsText(data)
	prompt := ReportPromptForType(reportType, dataText)

	// Resolve provider and model.
	prov := r.provider.Active()
	if prov == nil {
		r.failReport(reportID, reportType, title, "no active AI provider configured")
		return reportID, fmt.Errorf("no active AI provider configured")
	}

	model := r.reportModelForProvider(prov.Name())

	// Call LLM with timeout.
	llmCtx, cancel := context.WithTimeout(ctx, r.maxTime)
	defer cancel()

	resp, err := prov.Complete(llmCtx, &CompletionRequest{
		Model: model,
		Messages: []Message{
			{Role: "user", Content: prompt},
		},
	})
	if err != nil {
		r.failReport(reportID, reportType, title, fmt.Sprintf("LLM call failed: %v", err))
		return reportID, fmt.Errorf("LLM call: %w", err)
	}

	genTimeMs := time.Since(genStart).Milliseconds()

	// Build summary from stripped markdown.
	stripped := StripMarkdown(resp.Content)
	summary := truncateRunes(stripped, 200)

	// Complete the report (and supersede old one if applicable).
	if err := r.store.CompleteReportInTx(
		reportID,
		resp.Content,
		summary,
		prov.Name(),
		model,
		resp.TokenUsage.TotalTokens,
		genTimeMs,
		supersedeID,
	); err != nil {
		r.failReport(reportID, reportType, title, fmt.Sprintf("save report failed: %v", err))
		return reportID, fmt.Errorf("complete report: %w", err)
	}

	r.broadcast(map[string]interface{}{
		"type":               "ai_report_completed",
		"report_id":          reportID,
		"report_type":        reportType,
		"title":              title,
		"generation_time_ms": genTimeMs,
	})

	log.Printf("[ai] report %d (%s) generated in %dms, tokens=%d",
		reportID, reportType, genTimeMs, resp.TokenUsage.TotalTokens)

	return reportID, nil
}

// failReport marks a report as failed and broadcasts the failure.
func (r *Reporter) failReport(reportID int64, reportType, title, errMsg string) {
	if err := r.store.UpdateReportStatus(reportID, "failed", errMsg); err != nil {
		log.Printf("[ai] failed to update report %d status: %v", reportID, err)
	}
	r.broadcast(map[string]interface{}{
		"type":        "ai_report_failed",
		"report_id":   reportID,
		"report_type": reportType,
		"title":       title,
		"error":       errMsg,
	})
}

// broadcast sends a message to all admin WebSocket clients.
func (r *Reporter) broadcast(msg interface{}) {
	if r.hub != nil {
		r.hub.BroadcastAdmin(msg)
	}
}

// reportModelForProvider returns the report model name for a given provider.
func (r *Reporter) reportModelForProvider(name string) string {
	switch name {
	case "claude":
		return r.cfg.Claude.ReportModel
	case "openai":
		return r.cfg.OpenAI.ReportModel
	case "ollama":
		return r.cfg.Ollama.ReportModel
	default:
		return ""
	}
}

// ---------------------------------------------------------------------------
// CalcPeriod — default time windows
// ---------------------------------------------------------------------------

// CalcPeriod computes default period start/end (Unix timestamps) for the given
// report type relative to "now" in the provided timezone.
func CalcPeriod(reportType string, tz *time.Location) (start, end int64) {
	now := time.Now().In(tz)

	switch reportType {
	case "daily":
		// Yesterday 00:00:00 to 23:59:59
		yesterday := now.AddDate(0, 0, -1)
		s := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, tz)
		e := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 23, 59, 59, 0, tz)
		return s.Unix(), e.Unix()

	case "weekly":
		// Last Monday 00:00:00 to last Sunday 23:59:59
		weekday := now.Weekday()
		if weekday == time.Sunday {
			weekday = 7
		}
		// Days since last Monday = weekday - 1 (current week) + 7 (go back a full week)
		lastMonday := now.AddDate(0, 0, -int(weekday)-6)
		lastSunday := lastMonday.AddDate(0, 0, 6)
		s := time.Date(lastMonday.Year(), lastMonday.Month(), lastMonday.Day(), 0, 0, 0, 0, tz)
		e := time.Date(lastSunday.Year(), lastSunday.Month(), lastSunday.Day(), 23, 59, 59, 0, tz)
		return s.Unix(), e.Unix()

	case "monthly":
		// 1st of last month to last day of last month
		firstThisMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, tz)
		lastDayPrevMonth := firstThisMonth.Add(-time.Second) // last second of previous month
		s := time.Date(lastDayPrevMonth.Year(), lastDayPrevMonth.Month(), 1, 0, 0, 0, 0, tz)
		e := time.Date(lastDayPrevMonth.Year(), lastDayPrevMonth.Month(), lastDayPrevMonth.Day(), 23, 59, 59, 0, tz)
		return s.Unix(), e.Unix()

	case "quarterly":
		// First day of last quarter to last day of last quarter
		currentQuarter := (int(now.Month()) - 1) / 3 // 0-based: Q1=0, Q2=1, Q3=2, Q4=3
		var startMonth time.Month
		var startYear int
		if currentQuarter == 0 {
			// Current Q1 → last quarter is Q4 of previous year
			startMonth = time.October
			startYear = now.Year() - 1
		} else {
			startMonth = time.Month((currentQuarter-1)*3 + 1)
			startYear = now.Year()
		}
		endMonth := startMonth + 2
		s := time.Date(startYear, startMonth, 1, 0, 0, 0, 0, tz)
		// Last day of end month: go to first day of month after endMonth, subtract 1 second
		lastDay := time.Date(startYear, endMonth+1, 1, 0, 0, 0, 0, tz).Add(-time.Second)
		e := time.Date(lastDay.Year(), lastDay.Month(), lastDay.Day(), 23, 59, 59, 0, tz)
		return s.Unix(), e.Unix()

	case "yearly":
		// Last year Jan 1 to Dec 31
		lastYear := now.Year() - 1
		s := time.Date(lastYear, time.January, 1, 0, 0, 0, 0, tz)
		e := time.Date(lastYear, time.December, 31, 23, 59, 59, 0, tz)
		return s.Unix(), e.Unix()

	default:
		// Fall back to daily
		return CalcPeriod("daily", tz)
	}
}

// ---------------------------------------------------------------------------
// GenerateTitle — human-readable report titles
// ---------------------------------------------------------------------------

// GenerateTitle produces a human-readable Chinese title for the given report
// type and period start timestamp.
func GenerateTitle(reportType string, periodStart int64, tz *time.Location) string {
	t := time.Unix(periodStart, 0).In(tz)

	switch reportType {
	case "daily":
		return fmt.Sprintf("%d年%d月%d日 运维日报", t.Year(), t.Month(), t.Day())
	case "weekly":
		_, week := t.ISOWeek()
		return fmt.Sprintf("%d年第%d周 运维周报", t.Year(), week)
	case "monthly":
		return fmt.Sprintf("%d年%d月 运维月报", t.Year(), t.Month())
	case "quarterly":
		quarter := (int(t.Month())-1)/3 + 1
		return fmt.Sprintf("%d年第%d季度 运维季报", t.Year(), quarter)
	case "yearly":
		return fmt.Sprintf("%d年度 运维年报", t.Year())
	default:
		return fmt.Sprintf("%d年%d月%d日 运维日报", t.Year(), t.Month(), t.Day())
	}
}

// ---------------------------------------------------------------------------
// StripMarkdown — remove common markdown syntax
// ---------------------------------------------------------------------------

// StripMarkdown removes common Markdown syntax characters from text, keeping
// only the readable content. Useful for generating plain-text summaries.
func StripMarkdown(md string) string {
	// Process line by line to handle line-level patterns.
	lines := strings.Split(md, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip table separator lines (e.g. |---|---|)
		if isTableSeparator(trimmed) {
			continue
		}

		// Remove heading markers
		trimmed = strings.TrimLeft(trimmed, "# ")

		// Remove inline formatting
		trimmed = strings.ReplaceAll(trimmed, "**", "")
		trimmed = strings.ReplaceAll(trimmed, "__", "")
		trimmed = strings.ReplaceAll(trimmed, "``", "")
		trimmed = strings.ReplaceAll(trimmed, "`", "")
		trimmed = strings.ReplaceAll(trimmed, "*", "")

		// Remove table pipes
		trimmed = strings.ReplaceAll(trimmed, "|", " ")

		// Remove link syntax: [text](url) → text
		trimmed = removeLinkSyntax(trimmed)

		// Remove remaining brackets
		trimmed = strings.ReplaceAll(trimmed, "[", "")
		trimmed = strings.ReplaceAll(trimmed, "]", "")

		// Collapse whitespace
		trimmed = collapseSpaces(trimmed)
		trimmed = strings.TrimSpace(trimmed)

		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return strings.Join(result, " ")
}

// isTableSeparator detects Markdown table separator lines like |---|---|.
func isTableSeparator(line string) bool {
	cleaned := strings.ReplaceAll(line, "|", "")
	cleaned = strings.ReplaceAll(cleaned, "-", "")
	cleaned = strings.ReplaceAll(cleaned, ":", "")
	cleaned = strings.TrimSpace(cleaned)
	return cleaned == "" && strings.Contains(line, "---")
}

// removeLinkSyntax converts [text](url) to text.
func removeLinkSyntax(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if s[i] == '[' {
			// Find closing ]
			j := strings.Index(s[i:], "](")
			if j >= 0 {
				text := s[i+1 : i+j]
				// Skip past ](url)
				k := strings.Index(s[i+j:], ")")
				if k >= 0 {
					result.WriteString(text)
					i = i + j + k + 1
					continue
				}
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// collapseSpaces replaces runs of whitespace with a single space.
func collapseSpaces(s string) string {
	var b strings.Builder
	prev := false
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if !prev {
				b.WriteRune(' ')
				prev = true
			}
		} else {
			b.WriteRune(r)
			prev = false
		}
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// truncateRunes — take first n Unicode runes
// ---------------------------------------------------------------------------

// truncateRunes returns the first n runes of s. If s has fewer than n runes,
// it is returned unchanged.
func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	runes := []rune(s)
	return string(runes[:n])
}
