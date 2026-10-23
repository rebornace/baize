package middleware

import "time"

// JobKind identifies how a queued job should be executed.
type JobKind string

const (
	// KindRun is a normal agent run (chat / webhook / channel message).
	KindRun JobKind = "run"
	// KindHitlResume continues a waiting_human run after an approve/reject
	// decision (worker-side ContinueFromHITL; does not hold a slot while waiting).
	KindHitlResume JobKind = "hitl_resume"
	// KindChildReceipt wakes a parent waiting_child run with a structured receipt.
	KindChildReceipt JobKind = "child_receipt"
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
	Locale     string    `json:"locale,omitempty"`
	EnqueuedAt time.Time `json:"enqueued_at"`
	// HITL fields are set when Kind == KindHitlResume.
	HITLApprove    bool   `json:"hitl_approve,omitempty"`
	HITLComment    string `json:"hitl_comment,omitempty"`
	HITLRole       string `json:"hitl_role,omitempty"`
	HITLOperatorID string `json:"hitl_operator_id,omitempty"`
	// ChildReceiptJSON is set when Kind == KindChildReceipt (parent wake).
	ChildReceiptJSON string `json:"child_receipt_json,omitempty"`
}

// Part is a serializable multimodal content part (text or image data URI).
// It mirrors llm.ContentPart for transport across drivers; images ride as
// pre-built data: URIs so the payload stays queue-serializable.
type Part struct {
	Type    string `json:"type"` // "text" | "image"
	Text    string `json:"text,omitempty"`
	DataURL string `json:"data_url,omitempty"` // Type == "image": data: URI
}
