package config

import (
	"fmt"
	"os"
	"strings"

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
	Skills struct {
		BuiltinDir  string   `yaml:"builtin_dir"`
		BuiltinDirs []string `yaml:"builtin_dirs"`
		UserDir     string   `yaml:"user_dir"`
	} `yaml:"skills"`
	Agent struct {
		ID     string   `yaml:"id"`
		System string   `yaml:"system"`
		Skills []string `yaml:"skills"`
	} `yaml:"agent"`
	Connector struct {
		ID                      string   `yaml:"id"`
		Type                    string   `yaml:"type"`
		Spec                    string   `yaml:"spec"`
		BaseURL                 string   `yaml:"base_url"`
		ExecutionCallbackURL    string   `yaml:"execution_callback_url"`
		RequireApproval         []string `yaml:"require_approval"`
		RequireApprovalMutating bool     `yaml:"require_approval_mutating"`
		RequireLogin            []string `yaml:"require_login"`
		Auth struct {
			Mode        string `yaml:"mode"`
			Static      struct {
				Headers map[string]string `yaml:"headers"`
			} `yaml:"static"`
			Passthrough struct {
				Headers []string `yaml:"headers"`
			} `yaml:"passthrough"`
			VaultRef struct {
				Headers map[string]string `yaml:"headers"`
			} `yaml:"vault_ref"`
			Capture struct {
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
	Conversation struct {
		MaxMessages       int   `yaml:"max_messages"`
		PersistIdentities *bool `yaml:"persist_identities"`
	} `yaml:"conversation"`
	MockTicket struct {
		Listen string `yaml:"listen"` // :18080；off 关闭 mock-ticket
	} `yaml:"mock_ticket"`
	ControlPlane struct {
		OperatorToken string `yaml:"operator_token"` // 空=未配；env:VAR 或 file:/path 或明文
		AdminToken    string `yaml:"admin_token"`    // 空=未配；同上
	} `yaml:"control_plane"`
	Events struct {
		Webhook struct {
			URL     string            `yaml:"url"`
			Headers map[string]string `yaml:"headers"`
		} `yaml:"webhook"`
	} `yaml:"events"`
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
		cfg.Run.MaxSteps = 16
	}
	if cfg.Listen == "" {
		cfg.Listen = ":8080"
	}
	if cfg.MockTicket.Listen == "" {
		cfg.MockTicket.Listen = ":18080"
	}
	if cfg.Connector.Type == "" {
		cfg.Connector.Type = "openapi"
	}
	// require_approval 默认不注入；样板 configs/demo.yaml 自行列出 create_ticket 等
	if cfg.Store.Driver == "sqlite" && cfg.Store.SQLitePath == "" {
		cfg.Store.SQLitePath = "./data/baize.db"
	}
	// conversation.max_messages 默认 40（<=0 或缺省均回填）。
	// PersistIdentities 用 *bool 区分「未设置」与「显式 false」：
	// nil → 默认 true（由 bootstrap 按 driver 解析）；显式 false → 关闭持久化。
	if cfg.Conversation.MaxMessages <= 0 {
		cfg.Conversation.MaxMessages = 40
	}
	if cfg.Skills.UserDir == "" {
		cfg.Skills.UserDir = "./data/skills"
	}
	return cfg, nil
}

// SkillBuiltinDirs returns builtin skill scan roots. builtin_dirs wins over builtin_dir when set.
func (c *Config) SkillBuiltinDirs() []string {
	if len(c.Skills.BuiltinDirs) > 0 {
		out := make([]string, 0, len(c.Skills.BuiltinDirs))
		for _, d := range c.Skills.BuiltinDirs {
			d = strings.TrimSpace(d)
			if d != "" {
				out = append(out, d)
			}
		}
		return out
	}
	if d := strings.TrimSpace(c.Skills.BuiltinDir); d != "" {
		return []string{d}
	}
	return nil
}
