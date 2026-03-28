package config

import (
	"os"
	"os/exec"

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

// rawCollectConfig uses pointers to distinguish "not set" from "explicitly false".
type rawCollectConfig struct {
	Interval int   `yaml:"interval"`
	Docker   *bool `yaml:"docker"`
	GPU      *bool `yaml:"gpu"`
}

type rawConfig struct {
	Server  ServerConfig     `yaml:"server"`
	Collect rawCollectConfig `yaml:"collect"`
	Agent   AgentConfig      `yaml:"agent"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	cfg := &Config{
		Server: raw.Server,
		Agent:  raw.Agent,
		Collect: CollectConfig{
			Interval: raw.Collect.Interval,
		},
	}

	if cfg.Collect.Interval <= 0 {
		cfg.Collect.Interval = 5
	}

	// Docker: if not explicitly set, auto-detect by checking socket
	if raw.Collect.Docker != nil {
		cfg.Collect.Docker = *raw.Collect.Docker
	} else {
		_, err := os.Stat("/var/run/docker.sock")
		cfg.Collect.Docker = err == nil
	}

	// GPU: if not explicitly set, auto-detect by checking nvidia-smi
	if raw.Collect.GPU != nil {
		cfg.Collect.GPU = *raw.Collect.GPU
	} else {
		_, err := exec.LookPath("nvidia-smi")
		cfg.Collect.GPU = err == nil
	}

	return cfg, nil
}
