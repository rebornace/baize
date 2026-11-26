package conversation

import (
	"net/url"
	"path"
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
	ID               string    `json:"id"`
	ConversationID   string    `json:"conversation_id"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	Thinking         string    `json:"thinking,omitempty"`
	ThinkingRedacted bool      `json:"thinking_redacted,omitempty"`
	RunID            string    `json:"run_id"`
	CreatedAt        time.Time `json:"created_at"`
	// ReplyToID points at an earlier message in the same conversation.
	ReplyToID string `json:"reply_to_id,omitempty"`
	// PinnedSkills is the skill activation set for this user turn (agent
	// defaults and/or @id mentions). Empty for assistant / system_note rows.
	PinnedSkills []string `json:"pinned_skills,omitempty"`
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

// mediaRefParts captures image alt / URL and file name / URL so exports can
// list attachment names without exposing the internal media URL.
var mediaRefParts = regexp.MustCompile(
	`(?m)^!\[([^\]]*)\]\((/v0/channels/media/[^)\s]+)\)|^\[file:([^\]]+)\]\((/v0/channels/media/[^)\s]+)\)`)

// MediaRef is the export-safe view of one chat attachment: a display name,
// never the internal /v0/channels/media URL or bytes.
type MediaRef struct {
	Filename string `json:"filename"`
}

// MediaRefs returns unique attachment names referenced by a persisted message.
func MediaRefs(content string) []MediaRef {
	matches := mediaRefParts.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]MediaRef, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		file := strings.TrimSpace(m[3])
		rawURL := m[2]
		if rawURL == "" {
			rawURL = m[4]
		}
		if file == "" {
			file = filenameFromMediaURL(rawURL)
		}
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		out = append(out, MediaRef{Filename: file})
	}
	return out
}

func filenameFromMediaURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	base := path.Base(u.Path)
	if unescaped, err := url.PathUnescape(base); err == nil {
		base = unescaped
	}
	if base == "." || base == "/" {
		return ""
	}
	return base
}

// StripMediaRefs removes UI-only attachment reference lines from a persisted
// message's content, returning the model-facing text. The originating turn
// injects attachment content via multimodal parts instead.
func StripMediaRefs(content string) string {
	stripped := mediaRefLine.ReplaceAllString(content, "")
	return strings.TrimRight(stripped, "\r\n")
}
