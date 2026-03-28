package logging

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxScanRows = 10000

type LogSearcher struct {
	store  *LogStore
	logDir string
}

func NewLogSearcher(store *LogStore, logDir string) *LogSearcher {
	return &LogSearcher{store: store, logDir: logDir}
}

type SearchResult struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Module    string    `json:"module"`
	Source    string    `json:"source"`
	Message   string    `json:"message"`
}

// Search performs a keyword search across full log content.
// 1. Query log_index with time/level/module/source filters (up to maxScanRows)
// 2. For each candidate, check message_preview first (fast path)
// 3. If preview doesn't match, read original file line and check full msg
func (s *LogSearcher) Search(q LogQuery, keyword string) ([]SearchResult, error) {
	if keyword == "" {
		return nil, fmt.Errorf("keyword is required for search")
	}

	keyword = strings.ToLower(keyword)

	// Get candidates from index (up to maxScanRows)
	q.Page = 1
	q.PageSize = maxScanRows
	candidates, _, err := s.store.QueryLogIndex(q)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	for _, c := range candidates {
		// Fast path: check preview
		if strings.Contains(strings.ToLower(c.MessagePreview), keyword) {
			results = append(results, SearchResult{
				Timestamp: c.Timestamp,
				Level:     c.Level,
				Module:    c.Module,
				Source:    c.Source,
				Message:   c.MessagePreview,
			})
			continue
		}

		// Slow path: read full line from file
		fullMsg, err := s.readLogLine(c.FilePath, c.FileOffset, c.LineLength)
		if err != nil {
			continue // file might be cleaned up
		}
		if strings.Contains(strings.ToLower(fullMsg), keyword) {
			results = append(results, SearchResult{
				Timestamp: c.Timestamp,
				Level:     c.Level,
				Module:    c.Module,
				Source:    c.Source,
				Message:   fullMsg,
			})
		}
	}

	return results, nil
}

func (s *LogSearcher) readLogLine(relPath string, offset int64, length int) (string, error) {
	fullPath := filepath.Join(s.logDir, relPath)
	f, err := os.Open(fullPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	buf := make([]byte, length)
	_, err = f.ReadAt(buf, offset)
	if err != nil {
		return "", err
	}

	// Parse JSON to extract msg field
	var entry map[string]string
	if err := json.Unmarshal(buf, &entry); err != nil {
		return string(buf), nil
	}
	return entry["msg"], nil
}
