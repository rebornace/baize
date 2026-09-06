package webhook

// Header names for the channel adapter wire protocol.
const (
	HeaderProtocol  = "X-Baize-Protocol"
	HeaderTimestamp = "X-Baize-Channel-Timestamp"
	HeaderSignature = "X-Baize-Channel-Signature"
	HeaderRunID     = "X-Baize-Run-Id"
	ProtocolVersion = "v0"
)

// Peer identifies a conversation party on the IM side.
type Peer struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

// Attachment is an inbound file, inline (base64) or a URL baize will fetch.
type Attachment struct {
	Name          string `json:"name"`
	MIME          string `json:"mime"`
	ContentBase64 string `json:"content_base64,omitempty"`
	URL           string `json:"url,omitempty"`
}

// InboundMessage is the adapter -> baize request body.
type InboundMessage struct {
	Event string `json:"event"` // "message"
	// Account is informational only in 2A: one webhook instance serves exactly
	// one IM account, so the conversation account is always the instance's
	// configured account (config.account / instance name) and this field is
	// ignored. It is accepted for forward compatibility with a future
	// multi-account management plane.
	Account        string       `json:"account"`
	Peer           Peer         `json:"peer"`
	Text           string       `json:"text"`
	Attachments    []Attachment `json:"attachments,omitempty"`
	ContextToken   string       `json:"context_token,omitempty"`
	IdempotencyKey string       `json:"idempotency_key,omitempty"`
}

// OutboundMedia is a file baize pushes to the adapter.
type OutboundMedia struct {
	Name          string `json:"name"`
	MIME          string `json:"mime"`
	ContentBase64 string `json:"content_base64"`
}

// OutboundMessage is the baize -> adapter request body.
type OutboundMessage struct {
	Kind           string          `json:"kind"` // assistant|operator|notify
	ConversationID string          `json:"conversation_id"`
	Account        string          `json:"account"`
	Peer           Peer            `json:"peer"`
	Text           string          `json:"text,omitempty"`
	Media          []OutboundMedia `json:"media,omitempty"`
	RunID          string          `json:"run_id,omitempty"`
	ContextToken   string          `json:"context_token,omitempty"`
}
