package llm

import "context"

// Streamer is an optional Provider capability for streaming chat completions.
// Callbacks receive cumulative full text (not per-token fragments).
type Streamer interface {
	ChatStream(ctx context.Context, messages []Message, tools []ToolSpec, onThink, onContent func(cumulative string)) (Message, error)
}
