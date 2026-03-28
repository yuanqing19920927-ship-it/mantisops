package ai

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"mantisops/server/internal/model"
	"mantisops/server/internal/store"
)

// ---------------------------------------------------------------------------
// Data structures
// ---------------------------------------------------------------------------

// ReportData holds all collected data for a report generation cycle.
type ReportData struct {
	Period            Period
	Servers           []ServerSummary
	Alerts            AlertSummary
	Probes            ProbeSummary
	TotalContainers   int
	RunningContainers int
}

// Period defines the time window for data collection.
type Period struct {
	Start time.Time
	End   time.Time
}

// ServerSummary contains aggregated metrics for a single server.
type ServerSummary struct {
	HostID    string
	Hostname  string
	IP        string
	Online    bool
	CPUAvg    float64
	CPUMax    float64
	MemAvg    float64
	MemMax    float64
	DiskUsage float64 // latest
	NetRxAvg  float64 // bytes/sec
	NetTxAvg  float64
}

// AlertSummary contains aggregated alert statistics for a period.
type AlertSummary struct {
	TotalFired    int
	TotalResolved int
	CurrentFiring int
	ByType        map[string]int
	BySeverity    map[string]int
	MTTR          float64 // mean time to resolve in minutes
}

// ProbeSummary contains probe status counts.
type ProbeSummary struct {
	Total int
	Up    int
	Down  int
}

// ---------------------------------------------------------------------------
// DataCollector
// ---------------------------------------------------------------------------

// DataCollector gathers data from VictoriaMetrics and SQLite for report
// generation and chat context.
type DataCollector struct {
	vmURL       string
	serverStore *store.ServerStore
	alertStore  *store.AlertStore
	httpClient  *http.Client
	timezone    *time.Location
}

// NewDataCollector creates a DataCollector. timezone is an IANA location
// string (e.g. "Asia/Shanghai"); invalid values fall back to Asia/Shanghai.
func NewDataCollector(vmURL string, serverStore *store.ServerStore, alertStore *store.AlertStore, timezone string) *DataCollector {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		loc, _ = time.LoadLocation("Asia/Shanghai")
	}
	return &DataCollector{
		vmURL:       strings.TrimRight(vmURL, "/"),
		serverStore: serverStore,
		alertStore:  alertStore,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		timezone:    loc,
	}
}

// ---------------------------------------------------------------------------
// Collect — main entry point
// ---------------------------------------------------------------------------

// Collect gathers all data required for a report of the given type.
func (dc *DataCollector) Collect(reportType string, start, end time.Time) (*ReportData, error) {
	servers, err := dc.serverStore.List()
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}

	step := dc.stepForReportType(reportType)

	data := &ReportData{
		Period: Period{Start: start, End: end},
	}

	data.Servers = dc.collectServerMetrics(servers, start, end, step)
	data.Alerts = dc.collectAlertSummary(start, end)
	data.Probes = dc.collectProbeSummary()

	// Container counts from latest VM data
	dc.collectContainerCounts(data, servers)

	return data, nil
}

// ---------------------------------------------------------------------------
// Server metrics
// ---------------------------------------------------------------------------

func (dc *DataCollector) collectServerMetrics(servers []model.Server, start, end time.Time, step string) []ServerSummary {
	summaries := make([]ServerSummary, 0, len(servers))
	for _, srv := range servers {
		s := ServerSummary{
			HostID:   srv.HostID,
			Hostname: srv.Hostname,
			IP:       srv.IPAddresses,
			Online:   srv.Status == "online",
		}

		// CPU
		cpuVals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_cpu_usage_percent{host_id="%s"}`, srv.HostID),
			start, end, step)
		if err == nil && len(cpuVals) > 0 {
			s.CPUAvg, s.CPUMax = avgMax(cpuVals)
		}

		// Memory
		memVals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_memory_usage_percent{host_id="%s"}`, srv.HostID),
			start, end, step)
		if err == nil && len(memVals) > 0 {
			s.MemAvg, s.MemMax = avgMax(memVals)
		}

		// Disk (latest value)
		diskVals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_disk_usage_percent{host_id="%s",mount="/"}`, srv.HostID),
			start, end, step)
		if err == nil && len(diskVals) > 0 {
			s.DiskUsage = diskVals[len(diskVals)-1]
		}

		// Network RX
		rxVals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_network_rx_bytes_per_sec{host_id="%s"}`, srv.HostID),
			start, end, step)
		if err == nil && len(rxVals) > 0 {
			s.NetRxAvg, _ = avgMax(rxVals)
		}

		// Network TX
		txVals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_network_tx_bytes_per_sec{host_id="%s"}`, srv.HostID),
			start, end, step)
		if err == nil && len(txVals) > 0 {
			s.NetTxAvg, _ = avgMax(txVals)
		}

		summaries = append(summaries, s)
	}
	return summaries
}

// ---------------------------------------------------------------------------
// VM query
// ---------------------------------------------------------------------------

// vmResponse mirrors the VictoriaMetrics JSON response structure.
type vmResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Metric map[string]string `json:"metric"`
			Values [][]interface{}   `json:"values"`
		} `json:"result"`
	} `json:"data"`
}

// queryVM executes a query_range request against VictoriaMetrics and returns
// the numeric values from the first result series.
func (dc *DataCollector) queryVM(query string, start, end time.Time, step string) ([]float64, error) {
	u := fmt.Sprintf("%s/api/v1/query_range?query=%s&start=%d&end=%d&step=%s",
		dc.vmURL,
		url.QueryEscape(query),
		start.Unix(),
		end.Unix(),
		step,
	)

	resp, err := dc.httpClient.Get(u)
	if err != nil {
		return nil, fmt.Errorf("vm query error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("vm read body: %w", err)
	}

	var vmResp vmResponse
	if err := json.Unmarshal(body, &vmResp); err != nil {
		return nil, fmt.Errorf("vm json parse: %w", err)
	}

	if vmResp.Status != "success" || len(vmResp.Data.Result) == 0 {
		return nil, nil
	}

	// Extract numeric values from the first result series.
	// Each value pair is [timestamp, "string_value"].
	series := vmResp.Data.Result[0].Values
	values := make([]float64, 0, len(series))
	for _, pair := range series {
		if len(pair) < 2 {
			continue
		}
		strVal, ok := pair[1].(string)
		if !ok {
			continue
		}
		v, err := strconv.ParseFloat(strVal, 64)
		if err != nil {
			continue
		}
		values = append(values, v)
	}

	return values, nil
}

// ---------------------------------------------------------------------------
// Alert summary
// ---------------------------------------------------------------------------

func (dc *DataCollector) collectAlertSummary(start, end time.Time) AlertSummary {
	summary := AlertSummary{
		ByType:     make(map[string]int),
		BySeverity: make(map[string]int),
	}

	since := start.Format(time.RFC3339)
	until := end.Format(time.RFC3339)

	// All events in the period
	events, err := dc.alertStore.QueryEvents("", nil, since, until, 10000, 0)
	if err != nil {
		log.Printf("[data_collector] query alert events: %v", err)
		return summary
	}

	var totalResolveMinutes float64
	var resolvedCount int

	for _, e := range events {
		if e.Status == "firing" {
			summary.TotalFired++
		}
		if e.Status == "resolved" {
			summary.TotalResolved++
		}

		// Count by rule name (as type proxy)
		summary.ByType[e.RuleName]++
		// Count by severity
		summary.BySeverity[e.Level]++

		// MTTR: compute time from fired_at to resolved_at
		if e.Status == "resolved" && e.ResolvedAt != nil {
			dur := e.ResolvedAt.Sub(e.FiredAt).Minutes()
			if dur > 0 {
				totalResolveMinutes += dur
				resolvedCount++
			}
		}
	}

	if resolvedCount > 0 {
		summary.MTTR = totalResolveMinutes / float64(resolvedCount)
	}

	// Count events that originally fired (both still firing and resolved)
	summary.TotalFired = 0
	for _, e := range events {
		summary.TotalFired++
		_ = e // all events in this window were fired
	}
	summary.TotalFired = len(events)

	// Current firing (not bounded by period — live count)
	firingEvents, err := dc.alertStore.ListFiringEvents()
	if err == nil {
		summary.CurrentFiring = len(firingEvents)
	}

	// Re-count resolved properly
	summary.TotalResolved = 0
	for _, e := range events {
		if e.Status == "resolved" {
			summary.TotalResolved++
		}
	}

	return summary
}

// ---------------------------------------------------------------------------
// Probe summary
// ---------------------------------------------------------------------------

func (dc *DataCollector) collectProbeSummary() ProbeSummary {
	summary := ProbeSummary{}

	// Count probe rules from SQLite via the server store's underlying DB
	db := dc.serverStore.DB()
	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM probe_rules WHERE enabled=1").Scan(&total); err != nil {
		log.Printf("[data_collector] count probe_rules: %v", err)
		return summary
	}
	summary.Total = total

	// Query latest probe status from VM for each probe
	rows, err := db.Query("SELECT id FROM probe_rules WHERE enabled=1")
	if err != nil {
		log.Printf("[data_collector] list probe_rules: %v", err)
		return summary
	}
	defer rows.Close()

	now := time.Now()
	for rows.Next() {
		var probeID int
		if err := rows.Scan(&probeID); err != nil {
			continue
		}
		vals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_probe_status{probe_id="%d"}`, probeID),
			now.Add(-5*time.Minute), now, "1m")
		if err != nil || len(vals) == 0 {
			continue
		}
		latest := vals[len(vals)-1]
		if latest >= 1.0 {
			summary.Up++
		} else {
			summary.Down++
		}
	}

	return summary
}

// ---------------------------------------------------------------------------
// Container counts
// ---------------------------------------------------------------------------

func (dc *DataCollector) collectContainerCounts(data *ReportData, servers []model.Server) {
	now := time.Now()
	for _, srv := range servers {
		// Total containers: count series for container_state metric
		vals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_container_state{host_id="%s"}`, srv.HostID),
			now.Add(-5*time.Minute), now, "1m")
		if err != nil || len(vals) == 0 {
			continue
		}
		// Each value represents a container state (1=running, 0=stopped).
		// For a simple count, we query using the latest snapshot.
		// Since query_range returns one series per container, we need a different approach.
		// Use count() to get total, sum() to get running.
	}

	// Total containers via count
	now2 := time.Now()
	totalVals, err := dc.queryVM(
		`count(mantisops_container_state)`,
		now2.Add(-5*time.Minute), now2, "1m")
	if err == nil && len(totalVals) > 0 {
		data.TotalContainers = int(totalVals[len(totalVals)-1])
	}

	runningVals, err := dc.queryVM(
		`count(mantisops_container_state == 1)`,
		now2.Add(-5*time.Minute), now2, "1m")
	if err == nil && len(runningVals) > 0 {
		data.RunningContainers = int(runningVals[len(runningVals)-1])
	}
}

// ---------------------------------------------------------------------------
// Step mapping
// ---------------------------------------------------------------------------

func (dc *DataCollector) stepForReportType(reportType string) string {
	switch reportType {
	case "daily":
		return "5m"
	case "weekly":
		return "1h"
	case "monthly":
		return "6h"
	case "quarterly":
		return "1d"
	case "yearly":
		return "1d"
	default:
		return "5m"
	}
}

// ---------------------------------------------------------------------------
// FormatAsText — structured text for LLM prompt injection
// ---------------------------------------------------------------------------

// FormatAsText converts ReportData into a structured text block suitable for
// embedding into an LLM prompt.
func (dc *DataCollector) FormatAsText(data *ReportData) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("=== 报告周期 ===\n"))
	b.WriteString(fmt.Sprintf("开始: %s\n", data.Period.Start.In(dc.timezone).Format("2006-01-02 15:04")))
	b.WriteString(fmt.Sprintf("结束: %s\n\n", data.Period.End.In(dc.timezone).Format("2006-01-02 15:04")))

	// Servers
	b.WriteString("=== 服务器概况 ===\n")
	b.WriteString(fmt.Sprintf("总计: %d 台\n\n", len(data.Servers)))

	onlineCount := 0
	for _, s := range data.Servers {
		if s.Online {
			onlineCount++
		}
	}
	b.WriteString(fmt.Sprintf("在线: %d / 离线: %d\n\n", onlineCount, len(data.Servers)-onlineCount))

	b.WriteString(fmt.Sprintf("%-20s %-8s %8s %8s %8s %8s %8s %12s %12s\n",
		"主机名", "状态", "CPU均值", "CPU峰值", "内存均值", "内存峰值", "磁盘", "网络入", "网络出"))
	b.WriteString(strings.Repeat("-", 108) + "\n")
	for _, s := range data.Servers {
		status := "在线"
		if !s.Online {
			status = "离线"
		}
		b.WriteString(fmt.Sprintf("%-20s %-8s %7.2f%% %7.2f%% %7.2f%% %7.2f%% %7.2f%% %10.2f B %10.2f B\n",
			s.Hostname, status, s.CPUAvg, s.CPUMax, s.MemAvg, s.MemMax, s.DiskUsage,
			s.NetRxAvg, s.NetTxAvg))
	}
	b.WriteString("\n")

	// Alerts
	b.WriteString("=== 告警概况 ===\n")
	b.WriteString(fmt.Sprintf("期间触发: %d, 已恢复: %d, 当前活跃: %d\n",
		data.Alerts.TotalFired, data.Alerts.TotalResolved, data.Alerts.CurrentFiring))
	if data.Alerts.MTTR > 0 {
		b.WriteString(fmt.Sprintf("平均恢复时间(MTTR): %.2f 分钟\n", data.Alerts.MTTR))
	}
	if len(data.Alerts.BySeverity) > 0 {
		b.WriteString("按级别: ")
		parts := make([]string, 0, len(data.Alerts.BySeverity))
		for k, v := range data.Alerts.BySeverity {
			parts = append(parts, fmt.Sprintf("%s=%d", k, v))
		}
		b.WriteString(strings.Join(parts, ", ") + "\n")
	}
	if len(data.Alerts.ByType) > 0 {
		b.WriteString("按类型: ")
		parts := make([]string, 0, len(data.Alerts.ByType))
		for k, v := range data.Alerts.ByType {
			parts = append(parts, fmt.Sprintf("%s=%d", k, v))
		}
		b.WriteString(strings.Join(parts, ", ") + "\n")
	}
	b.WriteString("\n")

	// Probes
	b.WriteString("=== 探针概况 ===\n")
	b.WriteString(fmt.Sprintf("总计: %d, 正常: %d, 异常: %d\n\n",
		data.Probes.Total, data.Probes.Up, data.Probes.Down))

	// Containers
	b.WriteString("=== 容器概况 ===\n")
	b.WriteString(fmt.Sprintf("总计: %d, 运行中: %d\n",
		data.TotalContainers, data.RunningContainers))

	return b.String()
}

// ---------------------------------------------------------------------------
// CollectChatContext — quick snapshot for chat
// ---------------------------------------------------------------------------

// CollectChatContext returns a concise text snapshot of current infrastructure
// status for use as chat context. It avoids heavy VM range queries, using only
// simple latest-value lookups.
func (dc *DataCollector) CollectChatContext() string {
	var b strings.Builder
	b.WriteString("=== 当前基础设施状态 ===\n")
	b.WriteString(fmt.Sprintf("时间: %s\n\n", time.Now().In(dc.timezone).Format("2006-01-02 15:04:05")))

	servers, err := dc.serverStore.List()
	if err != nil {
		b.WriteString(fmt.Sprintf("(获取服务器列表失败: %v)\n", err))
		return b.String()
	}

	onlineCount := 0
	for _, srv := range servers {
		if srv.Status == "online" {
			onlineCount++
		}
	}
	b.WriteString(fmt.Sprintf("服务器: %d 台 (在线 %d / 离线 %d)\n\n", len(servers), onlineCount, len(servers)-onlineCount))

	// Per-server quick metrics (latest value from a short window)
	now := time.Now()
	lookback := now.Add(-5 * time.Minute)
	for _, srv := range servers {
		name := srv.Hostname
		if srv.DisplayName != "" {
			name = srv.DisplayName
		}
		status := "在线"
		if srv.Status != "online" {
			status = "离线"
		}
		b.WriteString(fmt.Sprintf("  [%s] %s (%s)\n", status, name, srv.IPAddresses))

		if srv.Status != "online" {
			continue
		}

		// CPU
		cpuVals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_cpu_usage_percent{host_id="%s"}`, srv.HostID),
			lookback, now, "1m")
		if err == nil && len(cpuVals) > 0 {
			b.WriteString(fmt.Sprintf("    CPU: %.2f%%", cpuVals[len(cpuVals)-1]))
		}

		// Memory
		memVals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_memory_usage_percent{host_id="%s"}`, srv.HostID),
			lookback, now, "1m")
		if err == nil && len(memVals) > 0 {
			b.WriteString(fmt.Sprintf("  内存: %.2f%%", memVals[len(memVals)-1]))
		}

		// Disk
		diskVals, err := dc.queryVM(
			fmt.Sprintf(`mantisops_disk_usage_percent{host_id="%s",mount="/"}`, srv.HostID),
			lookback, now, "1m")
		if err == nil && len(diskVals) > 0 {
			b.WriteString(fmt.Sprintf("  磁盘: %.2f%%", diskVals[len(diskVals)-1]))
		}

		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Current firing alerts
	b.WriteString("=== 当前活跃告警 ===\n")
	firingEvents, err := dc.alertStore.ListFiringEvents()
	if err != nil {
		b.WriteString(fmt.Sprintf("(查询失败: %v)\n", err))
	} else if len(firingEvents) == 0 {
		b.WriteString("无活跃告警\n")
	} else {
		for _, e := range firingEvents {
			b.WriteString(fmt.Sprintf("  [%s] %s - %s (值: %.2f, 触发于: %s)\n",
				e.Level, e.RuleName, e.TargetLabel, e.Value,
				e.FiredAt.In(dc.timezone).Format("01-02 15:04")))
		}
	}
	b.WriteString("\n")

	// Probe status
	b.WriteString("=== 探针状态 ===\n")
	ps := dc.collectProbeSummary()
	b.WriteString(fmt.Sprintf("总计: %d, 正常: %d, 异常: %d\n", ps.Total, ps.Up, ps.Down))

	return b.String()
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// avgMax returns the average and maximum of a float64 slice.
func avgMax(vals []float64) (avg, max float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	max = -math.MaxFloat64
	var sum float64
	for _, v := range vals {
		sum += v
		if v > max {
			max = v
		}
	}
	avg = sum / float64(len(vals))
	return avg, max
}
