package model

import "time"

type ServerGroup struct {
	ID          int       `json:"id"`
	Name        string    `json:"name"`
	SortOrder   int       `json:"sort_order"`
	ServerCount int       `json:"server_count,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
