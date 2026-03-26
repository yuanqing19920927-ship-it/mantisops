package model

import "time"

type Asset struct {
	ID          int       `json:"id"`
	ServerID    int       `json:"server_id"`
	Name        string    `json:"name"`
	Category    string    `json:"category"`
	Description string    `json:"description"`
	TechStack   string    `json:"tech_stack"`
	Path        string    `json:"path"`
	Port        string    `json:"port"`
	Status      string    `json:"status"`
	ExtraInfo   string    `json:"extra_info"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}
