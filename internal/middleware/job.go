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

// Part is a serializable multimodal content part (text or image URL/data).
// It mirrors llm.ContentPart for transport across drivers.
type Part struct {
	Type     string `json:"type"` // "text" | "image"
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}
