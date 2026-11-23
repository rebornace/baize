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
	// CallbackEventURL, when non-empty, is advertised to the sidecar as
	// context.callback_urls.event. The Runtime computes it from a signed
	// short-lived token bound to RunID; empty means "no callback channel"
	// (e.g. no RunID, or PublicBase/secret unavailable).
	CallbackEventURL string
}

type InvokeResult struct {
	Content map[string]any
	IsError bool
}
