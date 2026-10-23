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

// ChatOnce runs one user→assistant turn on the default session.
func ChatOnce(userText string) (string, error) {
	return intma.ChatOnce(userText)
}

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

// ChatOnce runs one turn on this agent.
func (a *Agent) ChatOnce(userText string) (string, error) {
	return a.inner.ChatOnce(userText)
}

// Reset clears this agent's history.
func (a *Agent) Reset() { a.inner.Reset() }

// MessageCount returns this agent's message count.
func (a *Agent) MessageCount() int { return a.inner.MessageCount() }
