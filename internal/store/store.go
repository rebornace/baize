package store

import "time"

type Status string

const (
	StatusQueued       Status = "queued"
	StatusRunning      Status = "running"
	StatusSucceeded    Status = "succeeded"
	StatusFailed       Status = "failed"
	StatusWaitingHuman Status = "waiting_human"
)

type Agent struct {
	ID     string   `json:"id"`
	System string   `json:"system"`
	Skills []string `json:"skills,omitempty"`
}

func cloneAgent(a Agent) Agent {
	if a.Skills != nil {
		a.Skills = append([]string(nil), a.Skills...)
	}
	return a
}

type Connector struct {
	ID              string        `json:"id"`
	Type            string        `json:"type"` // openapi, mcp
	Spec            string        `json:"spec"`
	BaseURL         string        `json:"base_url"`
	RequireApproval []string      `json:"require_approval,omitempty"`
	RequireLogin    []string      `json:"require_login,omitempty"`
	Auth            ConnectorAuth `json:"auth,omitempty"`
	MCP             MCPConfig     `json:"mcp,omitempty"`
}

type MCPConfig struct {
	Transport string            `json:"transport"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
}

// Tool source constants describing how a tool row entered the catalog.
const (
	ToolSourceSpec   = "spec"
	ToolSourceExtra  = "extra"
	ToolSourcePlugin = "plugin"
	ToolSourceMCP    = "mcp"
)

// Tool is a single row in the connector tool catalog.
type Tool struct {
	ConnectorID     string         `json:"connector_id"`
	Name            string         `json:"name"`
	Source          string         `json:"source"`
	Enabled           bool           `json:"enabled"`
	Title             string         `json:"title,omitempty"`
	Description       string         `json:"description,omitempty"`
	DescriptionCustom bool           `json:"description_custom"`
	Method          string         `json:"method,omitempty"`
	Path            string         `json:"path,omitempty"`
	InputSchema     map[string]any `json:"input_schema,omitempty"`
	RequireLogin    bool           `json:"require_login"`
	RequireApproval bool           `json:"require_approval"`
	OperationID     string         `json:"operation_id,omitempty"`
}

// ConnectorAuth stores the connector auth configuration shape (mode + references),
// not resolved secrets. Mirrors internal/authcred.Config / internal/config Auth.
type ConnectorAuth struct {
	Mode        string       `json:"mode,omitempty"`
	Static      StaticAuth   `json:"static,omitempty"`
	Passthrough PassThruAuth `json:"passthrough,omitempty"`
	VaultRef    VaultRefAuth `json:"vault_ref,omitempty"`
	Capture     CaptureAuth  `json:"capture,omitempty"`
}

type CaptureAuth struct {
	ToolNameGlob   string   `json:"tool_name_glob,omitempty"`
	TokenJSONPaths []string `json:"token_json_paths,omitempty"`
	LabelJSONPaths []string `json:"label_json_paths,omitempty"`
	HeaderTemplate string   `json:"header_template,omitempty"`
	DefaultScheme  string   `json:"default_scheme,omitempty"`
}

type StaticAuth struct {
	Headers map[string]string `json:"headers,omitempty"`
}

type PassThruAuth struct {
	Headers []string `json:"headers,omitempty"`
}

type VaultRefAuth struct {
	Headers map[string]string `json:"headers,omitempty"`
}

type Event struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
}

// WebhookConfig is per-run webhook delivery overrides. Stored in webhook_json;
// never echoed on GET /v0/runs/{id} (json:"-").
type WebhookConfig struct {
	URL     string            `json:"url,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

// SettingKeyEventsWebhook is the settings KV key for global events webhook config.
const SettingKeyEventsWebhook = "events_webhook"

type Run struct {
	ID             string    `json:"id"`
	AgentID        string    `json:"agent_id"`
	Input          string    `json:"input"`
	Status         Status    `json:"status"`
	Output         string    `json:"output,omitempty"`
	Error          string    `json:"error,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
	ConversationID string    `json:"conversation_id,omitempty"`
	IdentityID     string    `json:"identity_id,omitempty"`
	// PassthroughHeaders carries per-run passthrough auth headers. They are
	// never serialized to JSON (json:"-") so they cannot leak via GET /runs/{id}
	// or events. SQLite persists them in a dedicated passthrough_json column.
	PassthroughHeaders map[string]string `json:"-"`
	// WebhookConfig carries per-run webhook URL/header overrides for outbound
	// event delivery. Never serialized to JSON (json:"-").
	WebhookConfig *WebhookConfig `json:"-"`
}

// CreateRunInput is the input for creating a new run.
type CreateRunInput struct {
	AgentID            string
	Input              string
	ConversationID     string
	IdentityID         string
	PassthroughHeaders map[string]string
	WebhookConfig      *WebhookConfig
}

type HITLPayload struct {
	Prompt    string         `json:"prompt"`
	ToolName  string         `json:"tool_name"`
	Arguments map[string]any `json:"arguments"`
}

// Store persists agents, connectors, runs, events, and HITL payloads.
type Store interface {
	UpsertAgent(Agent)
	GetAgent(id string) (Agent, error)
	ListAgents() []Agent
	UpsertConnector(Connector)
	GetConnector(id string) (Connector, error)
	CreateRun(in CreateRunInput) (*Run, error)
	GetRun(id string) (*Run, error)
	UpdateRun(id string, status Status, output, errMsg string) error
	AppendEvent(runID string, ev Event) error
	ListEvents(runID string) ([]Event, error)
	SetHITL(runID string, payload *HITLPayload) error
	GetHITL(runID string) (*HITLPayload, error)
	SetPassthroughHeaders(runID string, headers map[string]string) error

	ListConnectors() []Connector
	ListTools() []Tool
	ListToolsByConnector(id string) []Tool
	GetTool(name string) (Tool, error)
	UpsertTool(Tool)
	DeleteTool(name string) error
	ReplaceConnectorTools(connectorID string, tools []Tool)

	GetSetting(key string) (jsonRaw []byte, ok bool, err error)
	UpsertSetting(key string, jsonRaw []byte) error
}