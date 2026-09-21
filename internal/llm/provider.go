package llm

import (
	"context"
	"encoding/json"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// Usage is the real per-call token accounting reported by the provider
// (OpenAI-compatible usage object). Zero value means the provider did not
// report usage (e.g. mock or a gateway that omits it); callers should treat it
// as "unknown", not as a literal zero-cost call.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	// CachedTokens counts prompt tokens served from the provider's prefix
	// cache (cache hit). Zero means no cache was reported or nothing hit.
	CachedTokens int `json:"cached_tokens"`
}

// UnmarshalJSON accepts the two common cache-hit dialects:
//   - DeepSeek: prompt_cache_hit_tokens / prompt_cache_miss_tokens at the top
//     level of usage.
//   - OpenAI: prompt_tokens_details.cached_tokens.
func (u *Usage) UnmarshalJSON(b []byte) error {
	type alias Usage
	var raw struct {
		alias
		PromptCacheHitTokens int `json:"prompt_cache_hit_tokens"`
		PromptTokensDetails  *struct {
			CachedTokens int `json:"cached_tokens"`
		} `json:"prompt_tokens_details"`
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}
	*u = Usage(raw.alias)
	switch {
	case raw.PromptCacheHitTokens > 0:
		u.CachedTokens = raw.PromptCacheHitTokens
	case raw.PromptTokensDetails != nil && raw.PromptTokensDetails.CachedTokens > 0:
		u.CachedTokens = raw.PromptTokensDetails.CachedTokens
	}
	return nil
}

type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
	Thinking   string
	// ThinkingRedacted is set when the upstream withheld displayable thinking.
	ThinkingRedacted bool
	// Usage carries the provider-reported token accounting for this call.
	Usage Usage
	// Parts is an optional multimodal payload. When non-empty, providers encode
	// the message content as a structured array (text + image parts) instead of
	// a plain string. Callers are responsible for including any text they want
	// as a "text"-typed Part; Content is ignored when Parts is set.
	Parts []ContentPart
}

// ContentPart is one element of a multimodal Message.Parts payload.
//
//   - Type == "text":  Text carries the text fragment.
//   - Type == "image": ImageBytes + ImageMIME are encoded as a base64 data URI.
//     If DataURL is non-empty it is forwarded verbatim (caller-supplied data URI)
//     and ImageBytes/ImageMIME are ignored.
type ContentPart struct {
	Type       string // "text" | "image"
	Text       string // Type == "text"
	ImageMIME  string // Type == "image"
	ImageBytes []byte // Type == "image" (raw bytes)
	DataURL    string // Type == "image", optional pre-built data: URI
}

type ToolCall struct {
	ID        string
	Name      string
	Arguments map[string]any
}

type ToolSpec struct {
	Name        string
	Description string
	InputSchema map[string]any
}

type Provider interface {
	Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error)
	// SupportsVision reports whether the backing model can accept image parts.
	// Callers use this to decide whether to attach images or to text-only fallback.
	SupportsVision() bool
}
