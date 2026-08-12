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
	ID     string `json:"id"`
	System string `json:"system"`
}

type Connector struct {
	ID              string   `json:"id"`
	Type            string   `json:"type"` // openapi
	Spec            string   `json:"spec"`
	BaseURL         string   `json:"base_url"`
	RequireApproval []string `json:"require_approval,omitempty"`
}

type Event struct {
	Type      string         `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	Data      map[string]any `json:"data,omitempty"`
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
}

// CreateRunInput is the input for creating a new run.
type CreateRunInput struct {
	AgentID        string
	Input          string
	ConversationID string
	IdentityID     string
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
	UpsertConnector(Connector)
	GetConnector(id string) (Connector, error)
	CreateRun(in CreateRunInput) (*Run, error)
	GetRun(id string) (*Run, error)
	UpdateRun(id string, status Status, output, errMsg string) error
	AppendEvent(runID string, ev Event) error
	ListEvents(runID string) ([]Event, error)
	SetHITL(runID string, payload *HITLPayload) error
	GetHITL(runID string) (*HITLPayload, error)
}