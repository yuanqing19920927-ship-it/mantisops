package model

import "time"

type ProbeRule struct {
	ID          int       `json:"id"`
	ServerID    int       `json:"server_id"`
	Name        string    `json:"name"`
	Host        string    `json:"host"`
	Port        int       `json:"port"`
	Protocol    string    `json:"protocol"`
	IntervalSec int       `json:"interval_sec"`
	TimeoutSec  int       `json:"timeout_sec"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

type ProbeResult struct {
	RuleID    int     `json:"rule_id"`
	Name      string  `json:"name"`
	Host      string  `json:"host"`
	Port      int     `json:"port"`
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latency_ms"`
	CheckedAt int64   `json:"checked_at"`
	Error     string  `json:"error,omitempty"`
}
