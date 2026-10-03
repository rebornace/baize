package middleware

import "time"

// JobKind identifies how a queued job should be executed.
type JobKind string

const (
	// KindRun is a normal agent run (chat / webhook / channel message).
	KindRun JobKind = "run"
)

// Job is a unit of work: a run to execute. The DB run row is authoritative;
// these fields let a worker replay the run without reconstructing context, and
// carry non-persisted multimodal parts on the live path.
type Job struct {
	RunID      string    `json:"run_id"`
	Kind       JobKind   `json:"kind"`
	AgentID    string    `json:"agent_id"`
	Input      string    `json:"input"`
	Skills     []string  `json:"skills,omitempty"`
	UserParts  []Part    `json:"user_parts,omitempty"`
	EnqueuedAt time.Time `json:"enqueued_at"`
}

// Part is a serializable multimodal content part (text or image data URI).
// It mirrors llm.ContentPart for transport across drivers; images ride as
// pre-built data: URIs so the payload stays queue-serializable.
type Part struct {
	Type    string `json:"type"` // "text" | "image"
	Text    string `json:"text,omitempty"`
	DataURL string `json:"data_url,omitempty"` // Type == "image": data: URI
}
