// Package mobileagent is the public gomobile bind surface for MOBILE-RT.
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

// Approve continues after a needs_approval pause (executes pending http_get).
func Approve(callID string) string { return intma.Approve(callID) }

// Deny rejects a pending http_get without network I/O and continues.
func Deny(callID string) string { return intma.Deny(callID) }

// Reset clears in-memory history on the default session.
func Reset() { intma.Reset() }

// MessageCount returns the number of stored messages.
func MessageCount() int { return intma.MessageCount() }

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

// Approve pending http_get on this agent.
func (a *Agent) Approve(callID string) string { return a.inner.Approve(callID) }

// Deny pending http_get on this agent.
func (a *Agent) Deny(callID string) string { return a.inner.Deny(callID) }

// Reset clears this agent's history.
func (a *Agent) Reset() { a.inner.Reset() }

// MessageCount returns this agent's message count.
func (a *Agent) MessageCount() int { return a.inner.MessageCount() }
