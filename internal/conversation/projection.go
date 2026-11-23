package conversation

import (
	"encoding/json"
	"fmt"
	"time"
)

// ContextProjection is the model-facing overlay for a conversation. It never
// rewrites persisted messages; buildMessages may inject Pins/Summary, and the
// engine may stub dropped tool results in the in-memory ReAct transcript.
type ContextProjection struct {
	ConversationID string    `json:"conversation_id"`
	Pins           []string  `json:"pins,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	Dropped        []string  `json:"dropped_tool_call_ids,omitempty"`
	Revision       int       `json:"revision"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func encodeJSONList(items []string) string {
	if items == nil {
		items = []string{}
	}
	b, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func decodeJSONList(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil
	}
	return out
}

func validateProjection(p ContextProjection) error {
	if p.ConversationID == "" {
		return fmt.Errorf("conversation id required")
	}
	return nil
}
