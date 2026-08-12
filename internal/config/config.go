package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen string `yaml:"listen"`
	Store  struct {
		Driver     string `yaml:"driver"`
		SQLitePath string `yaml:"sqlite_path"`
	} `yaml:"store"`
	UI struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"ui"`
	LLM struct {
		Provider         string `yaml:"provider"`
		BaseURL          string `yaml:"base_url"`
		Model            string `yaml:"model"`
		APIKeyEnv        string `yaml:"api_key_env"` // 默认 BAIZE_API_KEY
		DisableThinking  bool   `yaml:"disable_thinking"` // DeepSeek V4：关闭 thinking 省 token
	} `yaml:"llm"`
	Agent struct {
		ID     string `yaml:"id"`
		System string `yaml:"system"`
	} `yaml:"agent"`
	Connector struct {
		ID                      string   `yaml:"id"`
		Type                    string   `yaml:"type"`
		Spec                    string   `yaml:"spec"`
		BaseURL                 string   `yaml:"base_url"`
		RequireApproval         []string `yaml:"require_approval"`
		RequireApprovalMutating bool     `yaml:"require_approval_mutating"`
		Auth                    struct {
			BearerEnv string `yaml:"bearer_env"` // env holding JWT or "Bearer …"；作 Headers 兜底
			Capture   struct {
				ToolNameGlob   string   `yaml:"tool_name_glob"`
				TokenJSONPaths []string `yaml:"token_json_paths"`
				LabelJSONPaths []string `yaml:"label_json_paths"`
				HeaderTemplate string   `yaml:"header_template"`
				DefaultScheme  string   `yaml:"default_scheme"` // 空则注册时若仅一个 security scheme 则填入
			} `yaml:"capture"`
		} `yaml:"auth"`
	} `yaml:"connector"`
	Run struct {
		MaxSteps int `yaml:"max_steps"`
	} `yaml:"run"`
	Demo struct {
		TicketListen string `yaml:"ticket_listen"` // :18080；off 关闭 mock-ticket
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
	// require_approval 默认不注入；样板 configs/demo.yaml 自行列出 create_ticket 等
	if cfg.Store.Driver == "sqlite" && cfg.Store.SQLitePath == "" {
		cfg.Store.SQLitePath = "./data/baize.db"
	}
	return cfg, nil
}
