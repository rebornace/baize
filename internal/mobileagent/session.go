package mobileagent

import (
	"encoding/json"
	"fmt"
)

const (
	sessionSnapshotVersion = 1
	sessionSnapshotMaxRaw  = 4 << 20
	sessionSnapshotMaxMsgs = 2000
)

type sessionSnapshot struct {
	Version              int           `json:"version"`
	MessageCount         int           `json:"message_count"`
	AllowPrivateNetworks bool          `json:"allow_private_networks"`
	Messages             []chatMessage `json:"messages"`
}

// ExportSession returns a portable snapshot of the conversation history.
// API keys, connector auth and pending approvals are never included.
func (a *Agent) ExportSession() string {
	a.mu.Lock()
	msgs := sanitizeSessionMessages(append([]chatMessage(nil), a.msgs...))
	allow := a.allowPrivateNetworks
	a.mu.Unlock()
	b, _ := json.Marshal(sessionSnapshot{
		Version:              sessionSnapshotVersion,
		MessageCount:         len(msgs),
		AllowPrivateNetworks: allow,
		Messages:             msgs,
	})
	return string(b)
}

// ExportSession on the default session.
func ExportSession() string { return defaultSession.ExportSession() }

// ImportSession replaces the conversation history from ExportSession output.
// It never restores API keys, connector auth, streams or pending approvals.
func (a *Agent) ImportSession(raw string) string {
	if len(raw) > sessionSnapshotMaxRaw {
		return errJSON("session snapshot too large")
	}
	var snap sessionSnapshot
	if err := json.Unmarshal([]byte(raw), &snap); err != nil {
		return errJSON("invalid session snapshot: " + err.Error())
	}
	if snap.Version != sessionSnapshotVersion {
		return errJSON(fmt.Sprintf("unsupported session snapshot version %d", snap.Version))
	}
	if len(snap.Messages) > sessionSnapshotMaxMsgs {
		return errJSON(fmt.Sprintf("session snapshot has too many messages (%d)", len(snap.Messages)))
	}
	if err := validateSessionMessages(snap.Messages); err != nil {
		return errJSON(err.Error())
	}

	a.mu.Lock()
	if a.stream != nil && a.stream.active() {
		a.mu.Unlock()
		return errJSON("stream in progress")
	}
	a.msgs = append([]chatMessage(nil), snap.Messages...)
	a.pending = nil
	a.mu.Unlock()
	b, _ := json.Marshal(map[string]any{
		"status":                 "done",
		"message_count":          len(snap.Messages),
		"allow_private_networks": snap.AllowPrivateNetworks,
	})
	return string(b)
}

// ImportSession on the default session.
func ImportSession(raw string) string { return defaultSession.ImportSession(raw) }

// sanitizeSessionMessages truncates at the first assistant tool call that has
// no matching tool result. A pending/interrupted call cannot be resumed after
// process death, and a dangling call would make the restored context invalid.
func sanitizeSessionMessages(msgs []chatMessage) []chatMessage {
	results := map[string]bool{}
	for _, m := range msgs {
		if m.Role == "tool" && m.ToolCallID != "" {
			results[m.ToolCallID] = true
		}
	}
	for i, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		for _, tc := range m.ToolCalls {
			if tc.ID == "" || !results[tc.ID] {
				return msgs[:i]
			}
		}
	}
	return msgs
}

func validateSessionMessages(msgs []chatMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	pending := map[string]bool{}
	for i, m := range msgs {
		switch m.Role {
		case "system", "user", "assistant", "tool":
		default:
			return fmt.Errorf("message %d has invalid role %q", i, m.Role)
		}
		if m.Role == "user" && m.Content == "" {
			return fmt.Errorf("message %d is an empty user turn", i)
		}
		if m.Role == "tool" {
			if m.ToolCallID == "" {
				return fmt.Errorf("message %d is a tool result without tool_call_id", i)
			}
			if !pending[m.ToolCallID] {
				return fmt.Errorf("message %d tool result has no matching call", i)
			}
			delete(pending, m.ToolCallID)
		}
		if m.Role == "assistant" {
			for _, tc := range m.ToolCalls {
				if tc.ID == "" {
					return fmt.Errorf("message %d has a tool call without id", i)
				}
				pending[tc.ID] = true
			}
		}
	}
	if len(pending) > 0 {
		return fmt.Errorf("session snapshot ends with %d unresolved tool call(s)", len(pending))
	}
	return nil
}
