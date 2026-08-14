package httpplugin

import "errors"

const (
	HeaderProtocol = "X-Baize-Protocol"
	HeaderRunID    = "X-Baize-Run-Id"
	ProtocolV0     = "v0"
)

var ErrInvalidPlugin = errors.New("invalid_plugin")

type ToolDesc struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	Annotations map[string]any `json:"annotations,omitempty"`
}

type InvokeMeta struct {
	RunID   string
	AgentID string
	Headers map[string]string
}

type InvokeResult struct {
	Content map[string]any
	IsError bool
}
