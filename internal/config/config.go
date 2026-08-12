package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen string `yaml:"listen"`
	LLM    struct {
		Provider  string `yaml:"provider"`
		BaseURL   string `yaml:"base_url"`
		Model     string `yaml:"model"`
		APIKeyEnv string `yaml:"api_key_env"` // 默认 BAIZE_API_KEY
	} `yaml:"llm"`
	Agent struct {
		ID     string `yaml:"id"`
		System string `yaml:"system"`
	} `yaml:"agent"`
	Connector struct {
		ID      string `yaml:"id"`
		Type    string `yaml:"type"`
		Spec    string `yaml:"spec"`
		BaseURL string `yaml:"base_url"`
	} `yaml:"connector"`
	Run struct {
		MaxSteps int `yaml:"max_steps"`
	} `yaml:"run"`
	Demo struct {
		TicketListen string `yaml:"ticket_listen"` // :18080
	} `yaml:"demo"`
}

// Load reads and unmarshals a YAML config file.
func Load(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.LLM.APIKeyEnv == "" {
		cfg.LLM.APIKeyEnv = "BAIZE_API_KEY"
	}
	if cfg.Run.MaxSteps <= 0 {
		cfg.Run.MaxSteps = 8
	}
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.Demo.TicketListen == "" {
		cfg.Demo.TicketListen = ":18080"
	}
	if cfg.Connector.Type == "" {
		cfg.Connector.Type = "openapi"
	}
	return cfg, nil
}
