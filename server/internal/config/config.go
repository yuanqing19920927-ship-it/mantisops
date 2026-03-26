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
}

type ServerConfig struct {
	HTTPAddr    string `yaml:"http_addr"`
	GRPCAddr    string `yaml:"grpc_addr"`
	GRPCTLSAddr string `yaml:"grpc_tls_addr"`
	TLSCert     string `yaml:"grpc_tls_cert"`
	TLSKey      string `yaml:"grpc_tls_key"`
	PSKToken    string `yaml:"psk_token"`
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
