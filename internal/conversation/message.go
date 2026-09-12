package conversation

import (
	"regexp"
	"strings"
	"time"
)

const (
	RoleUser       = "user"
	RoleAssistant  = "assistant"
	RoleSystemNote = "system_note"
)

type Message struct {
	ID             string    `json:"id"`
	ConversationID string    `json:"conversation_id"`
	Role           string    `json:"role"`
	Content        string    `json:"content"`
	RunID          string    `json:"run_id"`
	CreatedAt      time.Time `json:"created_at"`
}

// mediaRefLine matches a persisted user-message line that references a
// conversation-ACL-protected chat attachment, e.g.
//
//	![图片](/v0/channels/media/<conv>/<obj>.jpg)
//	[file:行程.docx](/v0/channels/media/<conv>/<obj>.docx)
//
// These lines are UI-only; they must never be fed verbatim to the model as
// historical context (the URL is an internal artifact and the bytes travel via
// multimodal parts / extraction on the originating turn).
var mediaRefLine = regexp.MustCompile(
	`(?m)^(?:!\[[^\]]*\]|\[file:[^\]]+\])\(/v0/channels/media/[^)\s]+\)\s*\r?\n?`)

// StripMediaRefs removes UI-only attachment reference lines from a persisted
// message's content, returning the model-facing text. The originating turn
// injects attachment content via multimodal parts instead.
func StripMediaRefs(content string) string {
	stripped := mediaRefLine.ReplaceAllString(content, "")
	return strings.TrimRight(stripped, "\r\n")
}
