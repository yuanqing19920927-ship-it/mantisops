package model

import "time"

type ProbeRule struct {
	ID           int       `json:"id"`
	ServerID     *int      `json:"server_id"`
	Name         string    `json:"name"`
	Host         string    `json:"host"`
	Port         int       `json:"port"`
	Protocol     string    `json:"protocol"`
	URL          string    `json:"url"`
	ExpectStatus int       `json:"expect_status"`
	ExpectBody   string    `json:"expect_body"`
	IntervalSec  int       `json:"interval_sec"`
	TimeoutSec   int       `json:"timeout_sec"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

type ProbeResult struct {
	RuleID        int     `json:"rule_id"`
	Name          string  `json:"name"`
	Host          string  `json:"host"`
	Port          int     `json:"port"`
	Status        string  `json:"status"`
	LatencyMs     float64 `json:"latency_ms"`
	CheckedAt     int64   `json:"checked_at"`
	Error         string  `json:"error,omitempty"`
	HttpStatus    int     `json:"http_status,omitempty"`
	SSLExpiryDays *int    `json:"ssl_expiry_days,omitempty"`
	SSLIssuer     string  `json:"ssl_issuer,omitempty"`
	SSLExpiryDate string  `json:"ssl_expiry_date,omitempty"`
}
