// Package mobileagent is the public gomobile bind surface for on-device chat.
// Implementation lives in internal/mobileagent; this package only re-exports
// bind-friendly APIs (gomobile cannot bind packages under internal/).
package mobileagent

import (
	intma "github.com/rebornace/baize/internal/mobileagent"
)

// Configure sets LLM API base, key, and model on the default session.
func Configure(apiBase, apiKey, model string) error {
	return intma.Configure(apiBase, apiKey, model)
}

// ChatOnce runs one user→assistant turn without tools (M0 shortcut).
func ChatOnce(userText string) (string, error) {
	return intma.ChatOnce(userText)
}

// Send runs a user turn with tools until done or needs_approval. Returns JSON.
func Send(userText string) string { return intma.Send(userText) }

// Approve continues after a needs_approval pause (executes the pending call).
func Approve(callID string) string { return intma.Approve(callID) }

// Deny rejects a pending call without network I/O and continues.
func Deny(callID string) string { return intma.Deny(callID) }

// Reset clears in-memory history on the default session.
func Reset() { intma.Reset() }

// MessageCount returns the number of stored messages.
func MessageCount() int { return intma.MessageCount() }

// FetchRemoteConfig GETs remote JSON config (api_base/model/optional api_key).
func FetchRemoteConfig(url string) string { return intma.FetchRemoteConfig(url) }

// ApplyRemoteConfigJSON merges config JSON into the in-memory session.
func ApplyRemoteConfigJSON(raw string) string { return intma.ApplyRemoteConfigJSON(raw) }

// HasAPIKey reports whether an API key is held in memory.
func HasAPIKey() bool { return intma.HasAPIKey() }

// ConfigureReadOnlyAPI replaces the on-device GET-only OpenAPI connector.
func ConfigureReadOnlyAPI(baseURL, authHeader, authValue, spec string) error {
	return intma.ConfigureReadOnlyAPI(baseURL, authHeader, authValue, spec)
}

// ConfigureReadOnlyAPIs replaces the set with a JSON array of connectors:
// [{"id":"tickets","base_url":"…","auth_header":"…","auth_value":"…","spec":"…",
// "allow_writes":false}]. Writes are opt-in per connector and still require
// approval before every call.
func ConfigureReadOnlyAPIs(configsJSON string) error {
	return intma.ConfigureReadOnlyAPIs(configsJSON)
}

// ClearReadOnlyAPI removes the connector tool set.
func ClearReadOnlyAPI() { intma.ClearReadOnlyAPI() }

// RemoteToolCount returns the number of configured connector tools.
func RemoteToolCount() int { return intma.RemoteToolCount() }

// RemoteWriteToolCount returns how many tools use POST/PUT/PATCH.
func RemoteWriteToolCount() int { return intma.RemoteWriteToolCount() }

// RemoteToolNames returns comma-separated connector tool names (empty if none).
func RemoteToolNames() string { return intma.RemoteToolNames() }

// Agent is a bindable session handle (optional; free functions use the default).
type Agent struct {
	inner *intma.Agent
}

// NewAgent returns a new session (gomobile prefers New* constructors).
func NewAgent() *Agent {
	return &Agent{inner: intma.New()}
}

// Configure configures this agent.
func (a *Agent) Configure(apiBase, apiKey, model string) error {
	return a.inner.Configure(apiBase, apiKey, model)
}

// ChatOnce runs one no-tools turn on this agent.
func (a *Agent) ChatOnce(userText string) (string, error) {
	return a.inner.ChatOnce(userText)
}

// Send with tools on this agent.
func (a *Agent) Send(userText string) string { return a.inner.Send(userText) }

// Approve the pending call on this agent.
func (a *Agent) Approve(callID string) string { return a.inner.Approve(callID) }

// Deny the pending call on this agent.
func (a *Agent) Deny(callID string) string { return a.inner.Deny(callID) }

// Reset clears this agent's history.
func (a *Agent) Reset() { a.inner.Reset() }

// MessageCount returns this agent's message count.
func (a *Agent) MessageCount() int { return a.inner.MessageCount() }

// FetchRemoteConfig on this agent.
func (a *Agent) FetchRemoteConfig(url string) string { return a.inner.FetchRemoteConfig(url) }

// ApplyRemoteConfigJSON on this agent.
func (a *Agent) ApplyRemoteConfigJSON(raw string) string {
	return a.inner.ApplyRemoteConfigJSON(raw)
}

// HasAPIKey on this agent.
func (a *Agent) HasAPIKey() bool { return a.inner.HasAPIKey() }

// ConfigureReadOnlyAPI on this agent.
func (a *Agent) ConfigureReadOnlyAPI(baseURL, authHeader, authValue, spec string) error {
	return a.inner.ConfigureReadOnlyAPI(baseURL, authHeader, authValue, spec)
}

// ConfigureReadOnlyAPIs on this agent (JSON array of connectors).
func (a *Agent) ConfigureReadOnlyAPIs(configsJSON string) error {
	return a.inner.ConfigureReadOnlyAPIs(configsJSON)
}

// ClearReadOnlyAPI on this agent.
func (a *Agent) ClearReadOnlyAPI() { a.inner.ClearReadOnlyAPI() }

// RemoteToolCount on this agent.
func (a *Agent) RemoteToolCount() int { return a.inner.RemoteToolCount() }

// RemoteWriteToolCount on this agent.
func (a *Agent) RemoteWriteToolCount() int { return a.inner.RemoteWriteToolCount() }

// RemoteToolNames on this agent.
func (a *Agent) RemoteToolNames() string { return a.inner.RemoteToolNames() }
