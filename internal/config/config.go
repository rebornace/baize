package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/rebornace/baize/internal/inbox"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Listen string `yaml:"listen"`
	Store  struct {
		Driver     string `yaml:"driver"`
		SQLitePath string `yaml:"sqlite_path"`
		DSN        string `yaml:"dsn"`
	} `yaml:"store"`
	UI struct {
		Enabled bool `yaml:"enabled"`
	} `yaml:"ui"`
	LLM struct {
		Provider        string `yaml:"provider"`
		BaseURL         string `yaml:"base_url"`
		Model           string `yaml:"model"`
		APIKeyEnv       string `yaml:"api_key_env"`      // 默认 BAIZE_API_KEY
		DisableThinking bool   `yaml:"disable_thinking"` // DeepSeek V4：关闭 thinking 省 token
		// SupportsVision declares whether the backing model accepts image parts.
		// Defaults to false; enable for vision-capable OpenAI-compatible models.
		SupportsVision bool `yaml:"supports_vision"`
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
		Auth                    struct {
			Mode   string `yaml:"mode"`
			Static struct {
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
		// ToolTimeoutSec is per-tool HTTP/invoke deadline (default 60).
		ToolTimeoutSec int `yaml:"tool_timeout_sec"`
	} `yaml:"run"`
	Conversation struct {
		MaxMessages       int   `yaml:"max_messages"`
		PersistIdentities *bool `yaml:"persist_identities"`
		// Context compaction (rolling summary). CompactEnabled defaults to
		// true (use *bool to distinguish unset from explicit false). Threshold
		// is the fraction of the model context that may be used before older
		// history is folded into a rolling summary (out-of-range => 0.8);
		// ReserveOutput reserves headroom for the answer; RecentMessages is the
		// number of newest messages always kept verbatim.
		CompactEnabled        *bool   `yaml:"compact_enabled"`
		CompactThreshold      float64 `yaml:"compact_threshold"`
		CompactReserveOutput  int     `yaml:"compact_reserve_output"`
		CompactRecentMessages int     `yaml:"compact_recent_messages"`
	} `yaml:"conversation"`
	Middleware struct {
		Driver               string `yaml:"driver"` // memory（默认） | redis
		WorkerConcurrency    int    `yaml:"worker_concurrency"`
		LeaseTTLSec          int    `yaml:"lease_ttl_sec"`
		ReconcileIntervalSec int    `yaml:"reconcile_interval_sec"`
		Redis                struct {
			Addr          string `yaml:"addr"`
			DB            int    `yaml:"db"`
			Username      string `yaml:"username"`
			PasswordEnv   string `yaml:"password_env"`
			Stream        string `yaml:"stream"`
			ConsumerGroup string `yaml:"consumer_group"`
			EventsChannel string `yaml:"events_channel"`
		} `yaml:"redis"`
	} `yaml:"middleware"`
	MockTicket struct {
		Listen string `yaml:"listen"` // :18080；off 关闭 mock-ticket
	} `yaml:"mock_ticket"`
	ControlPlane struct {
		OperatorToken string          `yaml:"operator_token"` // 空=未配；env:VAR 或 file:/path 或明文
		AdminToken    string          `yaml:"admin_token"`    // 空=未配；同上
		Operators     []OperatorEntry `yaml:"operators"`
	} `yaml:"control_plane"`
	Events struct {
		Webhook struct {
			URL     string            `yaml:"url"`
			Headers map[string]string `yaml:"headers"`
		} `yaml:"webhook"`
	} `yaml:"events"`
	// Runtime governs sidecar callback_urls injection (Phase 2).
	// PublicBaseURL: base address advertised to sidecars; empty => do not inject
	//   callback URLs (avoid handing an unreachable address to a remote sidecar).
	// CallbackHMACSecret: HMAC key for short-lived callback tokens; empty =>
	//   bootstrap generates an ephemeral secret and logs "ephemeral".
	// CallbackTokenTTLSec: token lifetime; <=0 falls back to DefaultCallbackTokenTTLSec.
	Runtime   RuntimeConfig   `yaml:"runtime"`
	Inbox     InboxConfig     `yaml:"inbox"`
	MCPExport MCPExportConfig `yaml:"mcp_export"`
}

// MCPExportConfig toggles the Streamable HTTP MCP export surface.
type MCPExportConfig struct {
	// Enabled defaults to true when omitted (*bool distinguishes unset vs false).
	Enabled *bool `yaml:"enabled"`
}

// InboxConfig holds bootstrap defaults for inbound webhook channels.
type InboxConfig struct {
	Channels []inbox.Channel `yaml:"channels"`
}

// OperatorEntry is a named control-plane operator (id + token ref).
type OperatorEntry struct {
	ID    string `yaml:"id"`
	Token string `yaml:"token"`
}

// RuntimeConfig holds sidecar callback configuration (Phase 2).
type RuntimeConfig struct {
	PublicBaseURL       string `yaml:"public_base_url"`
	CallbackHMACSecret  string `yaml:"callback_hmac_secret"`
	CallbackTokenTTLSec int    `yaml:"callback_token_ttl_sec"`
}

// DefaultCallbackTokenTTLSec is the default callback token lifetime (1 hour),
// matching the v0 design spec §3.3.
const DefaultCallbackTokenTTLSec = 3600

// Load reads and unmarshals a YAML config file.
func Load(path string) (Config, error) {
	cfg, err := loadFile(path)
	if err != nil {
		return Config{}, err
	}
	applyDefaults(&cfg)
	return cfg, nil
}

// LoadLayered reads base then deep-merges each existing overlay file (later wins).
// Returns the merged config and the list of files actually applied (base + overlays).
func LoadLayered(basePath string, overlayPaths ...string) (Config, []string, error) {
	baseRaw, err := os.ReadFile(basePath)
	if err != nil {
		return Config{}, nil, fmt.Errorf("read config %s: %w", basePath, err)
	}
	merged, err := unmarshalYAMLMap(baseRaw)
	if err != nil {
		return Config{}, nil, fmt.Errorf("parse config %s: %w", basePath, err)
	}
	applied := []string{basePath}
	for _, path := range overlayPaths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return Config{}, nil, fmt.Errorf("read config %s: %w", path, err)
		}
		overlay, err := unmarshalYAMLMap(raw)
		if err != nil {
			return Config{}, nil, fmt.Errorf("parse config %s: %w", path, err)
		}
		deepMergeMap(merged, overlay)
		applied = append(applied, path)
	}
	cfg, err := configFromMap(merged)
	if err != nil {
		return Config{}, nil, err
	}
	applyDefaults(&cfg)
	return cfg, applied, nil
}

func loadFile(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	m, err := unmarshalYAMLMap(raw)
	if err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	return configFromMap(m)
}

func unmarshalYAMLMap(raw []byte) (map[string]interface{}, error) {
	var m map[string]interface{}
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	if m == nil {
		m = map[string]interface{}{}
	}
	return m, nil
}

func configFromMap(m map[string]interface{}) (Config, error) {
	out, err := yaml.Marshal(m)
	if err != nil {
		return Config{}, fmt.Errorf("marshal merged config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(out, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse merged config: %w", err)
	}
	return cfg, nil
}

func deepMergeMap(dst, src map[string]interface{}) {
	for k, srcVal := range src {
		dstVal, ok := dst[k]
		if !ok {
			dst[k] = srcVal
			continue
		}
		dstMap, dstOk := dstVal.(map[string]interface{})
		srcMap, srcOk := srcVal.(map[string]interface{})
		if dstOk && srcOk {
			deepMergeMap(dstMap, srcMap)
			continue
		}
		dst[k] = srcVal
	}
}

func applyDefaults(cfg *Config) {
	if cfg.LLM.APIKeyEnv == "" {
		cfg.LLM.APIKeyEnv = "BAIZE_API_KEY"
	}
	if cfg.Run.MaxSteps <= 0 {
		cfg.Run.MaxSteps = 16
	}
	if cfg.Run.ToolTimeoutSec <= 0 {
		cfg.Run.ToolTimeoutSec = 60
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
	if cfg.Conversation.CompactEnabled == nil {
		v := true
		cfg.Conversation.CompactEnabled = &v
	}
	if cfg.Conversation.CompactThreshold <= 0 || cfg.Conversation.CompactThreshold >= 1 {
		cfg.Conversation.CompactThreshold = 0.8
	}
	if cfg.Conversation.CompactReserveOutput <= 0 {
		cfg.Conversation.CompactReserveOutput = 8000
	}
	if cfg.Conversation.CompactRecentMessages <= 0 {
		cfg.Conversation.CompactRecentMessages = 8
	}
	if strings.TrimSpace(cfg.Middleware.Driver) == "" {
		cfg.Middleware.Driver = "memory"
	}
	if cfg.Middleware.WorkerConcurrency <= 0 {
		cfg.Middleware.WorkerConcurrency = 8
	}
	if cfg.Middleware.LeaseTTLSec <= 0 {
		cfg.Middleware.LeaseTTLSec = 60
	}
	if cfg.Middleware.ReconcileIntervalSec <= 0 {
		cfg.Middleware.ReconcileIntervalSec = 15
	}
	if cfg.Middleware.Redis.Stream == "" {
		cfg.Middleware.Redis.Stream = "baize:runs"
	}
	if cfg.Middleware.Redis.ConsumerGroup == "" {
		cfg.Middleware.Redis.ConsumerGroup = "baize-workers"
	}
	if cfg.Middleware.Redis.EventsChannel == "" {
		cfg.Middleware.Redis.EventsChannel = "baize:run-events"
	}
	if cfg.Skills.UserDir == "" {
		cfg.Skills.UserDir = "./data/skills"
	}
	if cfg.Runtime.CallbackTokenTTLSec <= 0 {
		cfg.Runtime.CallbackTokenTTLSec = DefaultCallbackTokenTTLSec
	}
	if cfg.MCPExport.Enabled == nil {
		v := true
		cfg.MCPExport.Enabled = &v
	}
}

// MCPExportEnabled reports whether MCP export is on (default true).
func (c Config) MCPExportEnabled() bool {
	if c.MCPExport.Enabled == nil {
		return true
	}
	return *c.MCPExport.Enabled
}

// CompactEnabled reports whether context compaction is on (default true).
func (c Config) CompactEnabled() bool {
	if c.Conversation.CompactEnabled == nil {
		return true
	}
	return *c.Conversation.CompactEnabled
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
