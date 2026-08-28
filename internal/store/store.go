package store

import (
	"errors"
	"fmt"
	"time"
)

type Status string

const (
	StatusQueued       Status = "queued"
	StatusRunning      Status = "running"
	StatusSucceeded    Status = "succeeded"
	StatusFailed       Status = "failed"
	StatusWaitingHuman Status = "waiting_human"
	StatusCancelled    Status = "cancelled"
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
	ID                   string        `json:"id"`
	Type                 string        `json:"type"` // openapi, http, mcp
	Spec                 string        `json:"spec"`
	ImportFormat         string        `json:"import_format_detected,omitempty"`
	BaseURL              string        `json:"base_url"`
	ExecutionCallbackURL string        `json:"execution_callback_url,omitempty"`
	RequireApproval      []string      `json:"require_approval,omitempty"`
	RequireLogin         []string      `json:"require_login,omitempty"`
	Auth                 ConnectorAuth `json:"auth,omitempty"`
	MCP                  MCPConfig     `json:"mcp,omitempty"`
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
	ConnectorID       string         `json:"connector_id"`
	Name              string         `json:"name"`
	Source            string         `json:"source"`
	Enabled           bool           `json:"enabled"`
	Title             string         `json:"title,omitempty"`
	Description       string         `json:"description,omitempty"`
	DescriptionCustom bool           `json:"description_custom"`
	Method            string         `json:"method,omitempty"`
	Path              string         `json:"path,omitempty"`
	InputSchema       map[string]any `json:"input_schema,omitempty"`
	RequireLogin      bool           `json:"require_login"`
	RequireApproval   bool           `json:"require_approval"`
	OperationID       string         `json:"operation_id,omitempty"`
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

// SettingKeyInboxChannels is the settings KV key for inbox channel configuration.
const SettingKeyInboxChannels = "inbox_channels"

// WebhookOutboxMaxAttempts is the fixed retry cap for outbound webhook deliveries.
const WebhookOutboxMaxAttempts = 5

// WebhookOutboxStatus is the lifecycle state of an outbound webhook delivery row.
type WebhookOutboxStatus string

const (
	WebhookOutboxPending   WebhookOutboxStatus = "pending"
	WebhookOutboxDelivered WebhookOutboxStatus = "delivered"
	WebhookOutboxDead      WebhookOutboxStatus = "dead"
)

// WebhookOutboxKind distinguishes event payloads from terminal run.ended posts.
type WebhookOutboxKind string

const (
	WebhookOutboxKindEvent WebhookOutboxKind = "event"
	WebhookOutboxKindEnded WebhookOutboxKind = "ended"
)

// WebhookOutboxEntry is a durable outbound webhook delivery attempt.
type WebhookOutboxEntry struct {
	ID           string
	DeliveryKey  string
	RunID        string
	Kind         WebhookOutboxKind
	EventIndex   int
	PayloadJSON  []byte
	TargetURL    string
	HeadersJSON  []byte
	Attempt      int
	MaxAttempts  int
	Status       WebhookOutboxStatus
	LastError    string
	NextRetryAt  time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// ErrWebhookOutboxNotFound is returned when a webhook outbox row is missing.
var ErrWebhookOutboxNotFound = errors.New("webhook outbox entry not found")

// WebhookOutboxDeliveryKey builds the deduplication key for a logical delivery.
func WebhookOutboxDeliveryKey(runID string, kind WebhookOutboxKind, eventIndex int) string {
	return fmt.Sprintf("%s:%s:%d", runID, kind, eventIndex)
}

// InboxDeliveryTTL is the idempotency retention window. GetInboxDelivery treats
// rows older than this as misses; PutInboxDelivery may overwrite expired rows.
const InboxDeliveryTTL = 24 * time.Hour

// ErrInboxDeliveryExists is returned by PutInboxDelivery when a fresh row
// already occupies the (channel_id, idempotency_key) slot.
var ErrInboxDeliveryExists = errors.New("inbox delivery already exists")

// InboxDelivery records an accepted inbound webhook delivery for idempotency.
type InboxDelivery struct {
	ChannelID, IdempotencyKey, DeliveryID, RunID, BodyHash string
	CreatedAt                                              time.Time
}

// InboxDeliveryFresh reports whether d is within InboxDeliveryTTL of now.
func InboxDeliveryFresh(d InboxDelivery, now time.Time) bool {
	if d.CreatedAt.IsZero() {
		return false
	}
	return now.Sub(d.CreatedAt) < InboxDeliveryTTL
}

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
	DeleteConnector(id string) error
	CreateRun(in CreateRunInput) (*Run, error)
	GetRun(id string) (*Run, error)
	UpdateRun(id string, status Status, output, errMsg string) error
	AppendEvent(runID string, ev Event) error
	ListEvents(runID string) ([]Event, error)
	SetHITL(runID string, payload *HITLPayload) error
	GetHITL(runID string) (*HITLPayload, error)
	SetPassthroughHeaders(runID string, headers map[string]string) error

	HasActiveRun(conversationID string) (bool, error)

	ListConnectors() []Connector
	ListTools() []Tool
	ListToolsByConnector(id string) []Tool
	GetTool(name string) (Tool, error)
	UpsertTool(Tool)
	DeleteTool(name string) error
	ReplaceConnectorTools(connectorID string, tools []Tool)

	GetSetting(key string) (jsonRaw []byte, ok bool, err error)
	UpsertSetting(key string, jsonRaw []byte) error

	GetInboxDelivery(channelID, idempotencyKey string) (InboxDelivery, bool, error)
	PutInboxDelivery(d InboxDelivery) error
	UpdateInboxDelivery(channelID, idempotencyKey, runID string) error
	GetInboxThread(channelID, externalID string) (conversationID string, ok bool, err error)
	PutInboxThread(channelID, externalID, conversationID string) error

	PutWebhookOutboxIfAbsent(entry WebhookOutboxEntry) (created bool, id string, err error)
	ListWebhookOutboxDue(now time.Time, limit int) ([]WebhookOutboxEntry, error)
	ListWebhookOutbox(statuses []WebhookOutboxStatus, limit int) ([]WebhookOutboxEntry, error)
	GetWebhookOutbox(id string) (WebhookOutboxEntry, error)
	UpdateWebhookOutbox(entry WebhookOutboxEntry) error
	ResetWebhookOutboxRetry(id string) error
}
