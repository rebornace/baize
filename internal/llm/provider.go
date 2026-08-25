package llm

import "context"

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

type Message struct {
	Role       Role
	Content    string
	ToolCallID string
	ToolCalls  []ToolCall
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
