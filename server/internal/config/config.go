package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server   ServerConfig   `yaml:"server"`
	Victoria VictoriaConfig `yaml:"victoria_metrics"`
	SQLite   SQLiteConfig   `yaml:"sqlite"`
	Probe    ProbeConfig    `yaml:"probe"`
	WS       WSConfig       `yaml:"websocket"`
	Aliyun   AliyunConfig   `yaml:"aliyun"`
	Auth          AuthConfig     `yaml:"auth"`
	EncryptionKey string         `yaml:"encryption_key"`
	AgentBin      AgentBinConfig `yaml:"agent"`
	Logging       LoggingConfig  `yaml:"logging"`
	AI            AIConfig       `yaml:"ai"`
}

type AuthConfig struct {
	Username  string `yaml:"username"`
	Password  string `yaml:"password"`
	JWTSecret string `yaml:"jwt_secret"`
}

type AliyunInstance struct {
	RegionID   string `yaml:"region_id"`
	InstanceID string `yaml:"instance_id"`
	HostID     string `yaml:"host_id"`
}

type AliyunRDS struct {
	InstanceID string `yaml:"instance_id"`
	HostID     string `yaml:"host_id"`
}

type AliyunConfig struct {
	Enabled         bool             `yaml:"enabled"`
	AccessKeyID     string           `yaml:"access_key_id"`
	AccessKeySecret string           `yaml:"access_key_secret"`
	Instances       []AliyunInstance `yaml:"instances"`
	RDS             []AliyunRDS      `yaml:"rds"`
	Interval        int              `yaml:"interval"`
}

type ServerConfig struct {
	HTTPAddr         string `yaml:"http_addr"`
	GRPCAddr         string `yaml:"grpc_addr"`
	GRPCAdvertiseAddr string `yaml:"grpc_advertise_addr"` // Agent 连接地址，空则用 GRPCAddr
	GRPCTLSAddr      string `yaml:"grpc_tls_addr"`
	TLSCert          string `yaml:"grpc_tls_cert"`
	TLSKey           string `yaml:"grpc_tls_key"`
	PSKToken         string `yaml:"psk_token"`
	StaticDir        string `yaml:"static_dir"`
}

type VictoriaConfig struct {
	URL string `yaml:"url"`
}

type SQLiteConfig struct {
	Path string `yaml:"path"`
}

type ProbeConfig struct {
	Interval int `yaml:"interval"`
	Timeout  int `yaml:"timeout"`
}

type WSConfig struct {
	PingInterval int `yaml:"ping_interval"`
}

type AgentBinConfig struct {
	BinaryDir       string `yaml:"binary_dir"`
	RegisterTimeout int    `yaml:"register_timeout"` // seconds, default 120
}

type LoggingConfig struct {
	Dir         string          `yaml:"dir"`
	Level       string          `yaml:"level"`
	BufferSize  int             `yaml:"buffer_size"`
	Retention   RetentionConfig `yaml:"retention"`
	CleanupHour int             `yaml:"cleanup_hour"`
}

type RetentionConfig struct {
	AuditDays  int `yaml:"audit_days"`
	SystemDays int `yaml:"system_days"`
	AgentDays  int `yaml:"agent_days"`
}

type AIConfig struct {
	Enabled        bool           `yaml:"enabled"`
	ActiveProvider string         `yaml:"active_provider"`
	Timezone       string         `yaml:"timezone"`
	Claude         ClaudeConfig   `yaml:"claude"`
	OpenAI         OpenAIConfig   `yaml:"openai"`
	Ollama         OllamaConfig   `yaml:"ollama"`
	Report         AIReportConfig `yaml:"report"`
	Chat           AIChatConfig   `yaml:"chat"`
}

type ClaudeConfig struct {
	APIKey      string `yaml:"api_key"`
	ReportModel string `yaml:"report_model"`
	ChatModel   string `yaml:"chat_model"`
	MaxTokens   int    `yaml:"max_tokens"`
}

type OpenAIConfig struct {
	APIKey      string `yaml:"api_key"`
	BaseURL     string `yaml:"base_url"`
	ReportModel string `yaml:"report_model"`
	ChatModel   string `yaml:"chat_model"`
	MaxTokens   int    `yaml:"max_tokens"`
}

type OllamaConfig struct {
	Host        string `yaml:"host"`
	ReportModel string `yaml:"report_model"`
	ChatModel   string `yaml:"chat_model"`
	MaxTokens   int    `yaml:"max_tokens"`
}

type AIReportConfig struct {
	MaxGenerationTime int `yaml:"max_generation_time"`
	MaxConcurrent     int `yaml:"max_concurrent"`
}

type AIChatConfig struct {
	MaxHistoryMessages int  `yaml:"max_history_messages"`
	MaxMessageLength   int  `yaml:"max_message_length"`
	SystemCtxRefresh   bool `yaml:"system_context_refresh"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	// Apply logging defaults
	if cfg.Logging.Dir == "" {
		cfg.Logging.Dir = "./logs"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Logging.BufferSize <= 0 {
		cfg.Logging.BufferSize = 4096
	}
	if cfg.Logging.Retention.AuditDays <= 0 {
		cfg.Logging.Retention.AuditDays = 90
	}
	if cfg.Logging.Retention.SystemDays <= 0 {
		cfg.Logging.Retention.SystemDays = 30
	}
	if cfg.Logging.Retention.AgentDays <= 0 {
		cfg.Logging.Retention.AgentDays = 7
	}
	if cfg.Logging.CleanupHour < 0 || cfg.Logging.CleanupHour > 23 {
		cfg.Logging.CleanupHour = 3
	}
	// Apply AI defaults
	if cfg.AI.Timezone == "" {
		cfg.AI.Timezone = "Asia/Shanghai"
	}
	if cfg.AI.Claude.ReportModel == "" {
		cfg.AI.Claude.ReportModel = "claude-sonnet-4-20250514"
	}
	if cfg.AI.Claude.ChatModel == "" {
		cfg.AI.Claude.ChatModel = "claude-haiku-4-5-20251001"
	}
	if cfg.AI.Claude.MaxTokens <= 0 {
		cfg.AI.Claude.MaxTokens = 8192
	}
	if cfg.AI.OpenAI.BaseURL == "" {
		cfg.AI.OpenAI.BaseURL = "https://api.openai.com/v1"
	}
	if cfg.AI.OpenAI.ReportModel == "" {
		cfg.AI.OpenAI.ReportModel = "gpt-4o"
	}
	if cfg.AI.OpenAI.ChatModel == "" {
		cfg.AI.OpenAI.ChatModel = "gpt-4o-mini"
	}
	if cfg.AI.OpenAI.MaxTokens <= 0 {
		cfg.AI.OpenAI.MaxTokens = 8192
	}
	if cfg.AI.Ollama.Host == "" {
		cfg.AI.Ollama.Host = "http://127.0.0.1:11434"
	}
	if cfg.AI.Ollama.MaxTokens <= 0 {
		cfg.AI.Ollama.MaxTokens = 4096
	}
	if cfg.AI.Report.MaxGenerationTime <= 0 {
		cfg.AI.Report.MaxGenerationTime = 300
	}
	if cfg.AI.Report.MaxConcurrent <= 0 {
		cfg.AI.Report.MaxConcurrent = 1
	}
	if cfg.AI.Chat.MaxHistoryMessages <= 0 {
		cfg.AI.Chat.MaxHistoryMessages = 20
	}
	if cfg.AI.Chat.MaxMessageLength <= 0 {
		cfg.AI.Chat.MaxMessageLength = 4000
	}
	return &cfg, nil
}
