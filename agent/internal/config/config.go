package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Collect CollectConfig `yaml:"collect"`
	Agent   AgentConfig   `yaml:"agent"`
}

type ServerConfig struct {
	Address string    `yaml:"address"`
	Token   string    `yaml:"token"`
	TLS     TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	Enabled bool   `yaml:"enabled"`
	CACert  string `yaml:"ca_cert"`
}

type CollectConfig struct {
	Interval int  `yaml:"interval"`
	Docker   bool `yaml:"docker"`
	GPU      bool `yaml:"gpu"`
}

type AgentConfig struct {
	ID string `yaml:"id"`
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
	if cfg.Collect.Interval <= 0 {
		cfg.Collect.Interval = 5
	}
	return &cfg, nil
}
