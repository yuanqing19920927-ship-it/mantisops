package api

import (
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"mantisops/server/internal/logging"
)

type LogHandler struct {
	store     *logging.LogStore
	searcher  *logging.LogSearcher
	permCache *PermissionCache
}

func NewLogHandler(store *logging.LogStore, searcher *logging.LogSearcher, pc *PermissionCache) *LogHandler {
	return &LogHandler{store: store, searcher: searcher, permCache: pc}
}

func (h *LogHandler) ListAudit(c *gin.Context) {
	q := logging.AuditQuery{
		Start:        parseTime(c.Query("start"), time.Now().Add(-time.Hour)),
		End:          parseTime(c.Query("end"), time.Now()),
		Username:     c.Query("username"),
		Action:       c.Query("action"),
		ResourceType: c.Query("resource_type"),
		Page:         parseInt(c.Query("page"), 1),
		PageSize:     parseInt(c.Query("page_size"), 50),
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}
	results, total, err := h.store.QueryAudit(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": results, "total": total, "page": q.Page, "page_size": q.PageSize})
}

func (h *LogHandler) ListRuntime(c *gin.Context) {
	q := logging.LogQuery{
		Start:    parseTime(c.Query("start"), time.Now().Add(-time.Hour)),
		End:      parseTime(c.Query("end"), time.Now()),
		Level:    c.Query("level"),
		Module:   c.Query("module"),
		Source:   c.Query("source"),
		Page:     parseInt(c.Query("page"), 1),
		PageSize: parseInt(c.Query("page_size"), 50),
	}
	if q.PageSize > 200 {
		q.PageSize = 200
	}

	// Permission check: non-admin can only see allowed sources
	ps := GetPermissionSet(c, h.permCache)
	if ps != nil && q.Source != "" && !ps.CanSeeLogSource(q.Source) {
		c.JSON(http.StatusOK, gin.H{"data": []interface{}{}, "total": 0, "page": q.Page, "page_size": q.PageSize})
		return
	}

	keyword := c.Query("keyword")
	if keyword != "" {
		results, err := h.searcher.Search(q, keyword)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if ps != nil {
			filtered := results[:0]
			for _, r := range results {
				if ps.CanSeeLogSource(r.Source) {
					filtered = append(filtered, r)
				}
			}
			results = filtered
		}
		c.JSON(http.StatusOK, gin.H{"data": results, "total": len(results)})
		return
	}

	results, total, err := h.store.QueryLogIndex(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ps != nil {
		filtered := results[:0]
		for _, r := range results {
			if ps.CanSeeLogSource(r.Source) {
				filtered = append(filtered, r)
			}
		}
		results = filtered
		total = len(filtered)
	}
	c.JSON(http.StatusOK, gin.H{"data": results, "total": total, "page": q.Page, "page_size": q.PageSize})
}

func (h *LogHandler) Sources(c *gin.Context) {
	sources, err := h.store.GetSources()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ps := GetPermissionSet(c, h.permCache); ps != nil {
		filtered := sources[:0]
		for _, s := range sources {
			if ps.CanSeeLogSource(s) {
				filtered = append(filtered, s)
			}
		}
		sources = filtered
	}
	c.JSON(http.StatusOK, sources)
}

func (h *LogHandler) Stats(c *gin.Context) {
	// TODO: add SQL-level permission filtering for non-admin users
	start := parseTime(c.Query("start"), time.Now().Add(-24*time.Hour))
	end := parseTime(c.Query("end"), time.Now())
	stats, err := h.store.GetStats(start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (h *LogHandler) Export(c *gin.Context) {
	format := c.DefaultQuery("format", "json")
	logType := c.DefaultQuery("type", "runtime")

	if logType == "audit" {
		// Audit export requires admin role (audit logs are admin-only)
		role, _ := c.Get("role")
		if role != "admin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
			return
		}
		q := logging.AuditQuery{
			Start:        parseTime(c.Query("start"), time.Now().Add(-time.Hour)),
			End:          parseTime(c.Query("end"), time.Now()),
			Username:     c.Query("username"),
			Action:       c.Query("action"),
			ResourceType: c.Query("resource_type"),
			Page:         1,
			PageSize:     10000,
		}
		results, _, err := h.store.QueryAudit(q)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		if format == "csv" {
			exportAuditCSV(c, results)
		} else {
			exportJSON(c, results, "audit-logs")
		}
		return
	}

	// runtime
	q := logging.LogQuery{
		Start:    parseTime(c.Query("start"), time.Now().Add(-time.Hour)),
		End:      parseTime(c.Query("end"), time.Now()),
		Level:    c.Query("level"),
		Module:   c.Query("module"),
		Source:   c.Query("source"),
		Page:     1,
		PageSize: 10000,
	}
	results, _, err := h.store.QueryLogIndex(q)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if format == "csv" {
		exportRuntimeCSV(c, results)
	} else {
		exportJSON(c, results, "runtime-logs")
	}
}

func exportAuditCSV(c *gin.Context, records []logging.AuditRecord) {
	c.Header("Content-Disposition", "attachment; filename=audit-logs.csv")
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF}) // UTF-8 BOM for Excel
	w := csv.NewWriter(c.Writer)
	w.Write([]string{"时间", "操作人", "操作", "资源类型", "资源ID", "资源名称", "详情", "IP", "UserAgent"})
	for _, r := range records {
		w.Write([]string{
			r.Timestamp.Format("2006-01-02 15:04:05"),
			r.Username, r.Action, r.ResourceType, r.ResourceID, r.ResourceName,
			r.Detail, r.IPAddress, r.UserAgent,
		})
	}
	w.Flush()
}

func exportRuntimeCSV(c *gin.Context, records []logging.LogIndexRecord) {
	c.Header("Content-Disposition", "attachment; filename=runtime-logs.csv")
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(c.Writer)
	w.Write([]string{"时间", "级别", "模块", "来源", "消息"})
	for _, r := range records {
		w.Write([]string{
			r.Timestamp.Format("2006-01-02 15:04:05"),
			r.Level, r.Module, r.Source, r.MessagePreview,
		})
	}
	w.Flush()
}

func exportJSON(c *gin.Context, data interface{}, filename string) {
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s.json", filename))
	c.Header("Content-Type", "application/json")
	c.JSON(http.StatusOK, data)
}

func parseTime(s string, def time.Time) time.Time {
	if s == "" {
		return def
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return def
}

func parseInt(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
