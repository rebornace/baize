package conversation

import "time"

// RollingSummary is the derived, per-conversation compaction state: a rolling
// summary of older messages plus a cursor pointing at the last summarized
// message. Raw messages are never deleted; this record can be dropped and
// recomputed at any time.
type RollingSummary struct {
	ConversationID         string    `json:"conversation_id"`
	Summary                string    `json:"summary"`
	CoversThroughMessageID string    `json:"covers_through_message_id"`
	CoversThroughOrder     int       `json:"covers_through_order"`
	UpdatedAt              time.Time `json:"updated_at"`
}
