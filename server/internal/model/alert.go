package model

import "time"

// AlertRule 告警规则
type AlertRule struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`       // server_offline/probe_down/cpu/memory/disk/container/gpu_temp/gpu_memory/network_rx/network_tx
	TargetID  string    `json:"target_id"`  // host_id 或 probe rule_id，空=全局
	Operator  string    `json:"operator"`   // > < >= <= == !=
	Threshold float64   `json:"threshold"`
	Unit      string    `json:"unit"`       // % / °C / MB/s / 秒
	Duration  int       `json:"duration"`   // 连续确认次数
	Level     string    `json:"level"`      // info/warning/critical
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// AlertEvent 告警事件
type AlertEvent struct {
	ID          int        `json:"id"`
	RuleID      int        `json:"rule_id"`
	RuleName    string     `json:"rule_name"`     // 快照
	TargetID    string     `json:"target_id"`
	TargetLabel string     `json:"target_label"`  // 快照：如 "yuanqing2 (192.168.10.65)"
	Level       string     `json:"level"`
	Status      string     `json:"status"`        // firing/resolved
	Silenced    bool       `json:"silenced"`
	Value       float64    `json:"value"`
	Message     string     `json:"message"`
	FiredAt     time.Time  `json:"fired_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	ResolveType string     `json:"resolve_type,omitempty"` // auto/target_gone/rule_disabled/rule_deleted
	AckedAt     *time.Time `json:"acked_at,omitempty"`
	AckedBy     string     `json:"acked_by,omitempty"`
}

// NotificationChannel 通知渠道
type NotificationChannel struct {
	ID        int       `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`    // dingtalk/webhook
	Config    string    `json:"config"`  // JSON
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

// AlertNotification 通知投递记录
type AlertNotification struct {
	ID          int        `json:"id"`
	EventID     int        `json:"event_id"`
	ChannelID   int        `json:"channel_id"`
	NotifyType  string     `json:"notify_type"`  // firing/resolved
	Status      string     `json:"status"`       // pending/sending/sent/failed
	RetryCount  int        `json:"retry_count"`
	LastError   string     `json:"last_error"`
	CreatedAt   time.Time  `json:"created_at"`
	ClaimedAt   *time.Time `json:"claimed_at,omitempty"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
}

// AlertNotificationDetail 通知投递详情（JOIN 查询结果）
type AlertNotificationDetail struct {
	ChannelName string     `json:"channel_name"`
	ChannelType string     `json:"channel_type"`
	NotifyType  string     `json:"notify_type"`
	Status      string     `json:"status"`
	RetryCount  int        `json:"retry_count"`
	LastError   string     `json:"last_error"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
}

// AlertStats 告警统计
type AlertStats struct {
	Firing           int `json:"firing"`
	FiringUnsilenced int `json:"firing_unsilenced"`
	TodayFired       int `json:"today_fired"`
	TodayResolved    int `json:"today_resolved"`
	TodaySilenced    int `json:"today_silenced"`
}
