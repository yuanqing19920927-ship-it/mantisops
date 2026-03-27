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

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
