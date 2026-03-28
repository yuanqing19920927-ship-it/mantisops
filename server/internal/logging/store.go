package logging

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

type LogStore struct {
	db *sql.DB
}

func NewLogStore(db *sql.DB) (*LogStore, error) {
	schema := []string{
		`CREATE TABLE IF NOT EXISTS audit_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			username TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL,
			resource_type TEXT NOT NULL,
			resource_id TEXT DEFAULT '',
			resource_name TEXT DEFAULT '',
			detail TEXT DEFAULT '',
			ip_address TEXT DEFAULT '',
			user_agent TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_timestamp ON audit_logs(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_username ON audit_logs(username, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_audit_resource ON audit_logs(resource_type, timestamp DESC)`,
		`CREATE TABLE IF NOT EXISTS log_index (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp DATETIME NOT NULL,
			level TEXT NOT NULL,
			module TEXT NOT NULL,
			source TEXT NOT NULL,
			file_path TEXT NOT NULL,
			file_offset INTEGER NOT NULL,
			line_length INTEGER NOT NULL,
			message_preview TEXT DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_log_timestamp ON log_index(timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_log_level ON log_index(level, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_log_module ON log_index(module, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_log_source ON log_index(source, timestamp DESC)`,
	}
	for _, ddl := range schema {
		if _, err := db.Exec(ddl); err != nil {
			return nil, fmt.Errorf("init log schema: %w", err)
		}
	}
	return &LogStore{db: db}, nil
}

func (s *LogStore) InsertAudit(ts time.Time, username, action, resourceType, resourceID, resourceName, detail, ip, ua string) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs (timestamp, username, action, resource_type, resource_id, resource_name, detail, ip_address, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ts.UTC(), username, action, resourceType, resourceID, resourceName, detail, ip, ua)
	return err
}

func (s *LogStore) InsertLogIndex(ts time.Time, level, module, source, filePath string, offset int64, lineLen int, preview string) error {
	_, err := s.db.Exec(`INSERT INTO log_index (timestamp, level, module, source, file_path, file_offset, line_length, message_preview)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ts.UTC(), level, module, source, filePath, offset, lineLen, preview)
	return err
}

// --- Query types ---

type AuditQuery struct {
	Start        time.Time
	End          time.Time
	Username     string
	Action       string
	ResourceType string
	Page         int
	PageSize     int
}

type AuditRecord struct {
	ID           int       `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Username     string    `json:"username"`
	Action       string    `json:"action"`
	ResourceType string    `json:"resource_type"`
	ResourceID   string    `json:"resource_id"`
	ResourceName string    `json:"resource_name"`
	Detail       string    `json:"detail"`
	IPAddress    string    `json:"ip_address"`
	UserAgent    string    `json:"user_agent"`
}

func (s *LogStore) QueryAudit(q AuditQuery) ([]AuditRecord, int, error) {
	where := []string{"timestamp BETWEEN ? AND ?"}
	args := []interface{}{q.Start.UTC(), q.End.UTC()}

	if q.Username != "" {
		where = append(where, "username = ?")
		args = append(args, q.Username)
	}
	if q.Action != "" {
		where = append(where, "action = ?")
		args = append(args, q.Action)
	}
	if q.ResourceType != "" {
		where = append(where, "resource_type = ?")
		args = append(args, q.ResourceType)
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM audit_logs WHERE "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if q.PageSize <= 0 {
		q.PageSize = 50
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	offset := (q.Page - 1) * q.PageSize
	queryArgs := append(append([]interface{}{}, args...), q.PageSize, offset)

	rows, err := s.db.Query(
		"SELECT id, timestamp, username, action, resource_type, resource_id, resource_name, detail, ip_address, user_agent FROM audit_logs WHERE "+whereClause+" ORDER BY timestamp DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []AuditRecord
	for rows.Next() {
		var r AuditRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Username, &r.Action, &r.ResourceType, &r.ResourceID, &r.ResourceName, &r.Detail, &r.IPAddress, &r.UserAgent); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, nil
}

type LogQuery struct {
	Start    time.Time
	End      time.Time
	Level    string // comma-separated
	Module   string
	Source   string
	Page     int
	PageSize int
}

type LogIndexRecord struct {
	ID             int       `json:"id"`
	Timestamp      time.Time `json:"timestamp"`
	Level          string    `json:"level"`
	Module         string    `json:"module"`
	Source         string    `json:"source"`
	FilePath       string    `json:"file_path"`
	FileOffset     int64     `json:"file_offset"`
	LineLength     int       `json:"line_length"`
	MessagePreview string    `json:"message_preview"`
}

func (s *LogStore) QueryLogIndex(q LogQuery) ([]LogIndexRecord, int, error) {
	where := []string{"timestamp BETWEEN ? AND ?"}
	args := []interface{}{q.Start.UTC(), q.End.UTC()}

	if q.Level != "" {
		levels := strings.Split(q.Level, ",")
		placeholders := strings.Repeat("?,", len(levels))
		placeholders = placeholders[:len(placeholders)-1]
		where = append(where, "level IN ("+placeholders+")")
		for _, l := range levels {
			args = append(args, strings.TrimSpace(l))
		}
	}
	if q.Module != "" {
		where = append(where, "module = ?")
		args = append(args, q.Module)
	}
	if q.Source != "" {
		where = append(where, "source = ?")
		args = append(args, q.Source)
	}

	whereClause := strings.Join(where, " AND ")

	var total int
	err := s.db.QueryRow("SELECT COUNT(*) FROM log_index WHERE "+whereClause, args...).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	if q.PageSize <= 0 {
		q.PageSize = 50
	}
	if q.Page <= 0 {
		q.Page = 1
	}
	offset := (q.Page - 1) * q.PageSize
	queryArgs := append(append([]interface{}{}, args...), q.PageSize, offset)

	rows, err := s.db.Query(
		"SELECT id, timestamp, level, module, source, file_path, file_offset, line_length, message_preview FROM log_index WHERE "+whereClause+" ORDER BY timestamp DESC LIMIT ? OFFSET ?",
		queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []LogIndexRecord
	for rows.Next() {
		var r LogIndexRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Level, &r.Module, &r.Source, &r.FilePath, &r.FileOffset, &r.LineLength, &r.MessagePreview); err != nil {
			return nil, 0, err
		}
		results = append(results, r)
	}
	return results, total, nil
}

func (s *LogStore) CleanupAuditBefore(before time.Time) (int64, error) {
	res, err := s.db.Exec("DELETE FROM audit_logs WHERE timestamp < ?", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *LogStore) CleanupLogIndexBefore(before time.Time) (int64, error) {
	res, err := s.db.Exec("DELETE FROM log_index WHERE timestamp < ?", before)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *LogStore) GetSources() ([]string, error) {
	rows, err := s.db.Query("SELECT DISTINCT source FROM log_index ORDER BY source")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var sources []string
	for rows.Next() {
		var src string
		rows.Scan(&src)
		sources = append(sources, src)
	}
	return sources, nil
}

func (s *LogStore) GetStats(start, end time.Time) (map[string]int, error) {
	stats := make(map[string]int)
	rows, err := s.db.Query("SELECT level, COUNT(*) FROM log_index WHERE timestamp BETWEEN ? AND ? GROUP BY level", start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var level string
		var count int
		rows.Scan(&level, &count)
		stats[level] = count
	}
	return stats, nil
}

// GetStatsFiltered returns log stats filtered to only visible sources.
func (s *LogStore) GetStatsFiltered(start, end time.Time, sources []string) (map[string]int, error) {
	if len(sources) == 0 {
		return make(map[string]int), nil
	}
	placeholders := make([]string, len(sources))
	args := []interface{}{start.UTC(), end.UTC()}
	for i, src := range sources {
		placeholders[i] = "?"
		args = append(args, src)
	}
	query := "SELECT level, COUNT(*) FROM log_index WHERE timestamp BETWEEN ? AND ? AND source IN (" + strings.Join(placeholders, ",") + ") GROUP BY level"
	stats := make(map[string]int)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var level string
		var count int
		rows.Scan(&level, &count)
		stats[level] = count
	}
	return stats, nil
}

// GetSourcesFiltered returns distinct sources filtered to only visible ones.
func (s *LogStore) GetSourcesFiltered(sources []string) ([]string, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	placeholders := make([]string, len(sources))
	args := make([]interface{}, len(sources))
	for i, src := range sources {
		placeholders[i] = "?"
		args[i] = src
	}
	query := "SELECT DISTINCT source FROM log_index WHERE source IN (" + strings.Join(placeholders, ",") + ") ORDER BY source"
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var src string
		rows.Scan(&src)
		result = append(result, src)
	}
	return result, nil
}
