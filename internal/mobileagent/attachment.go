package mobileagent

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

const (
	maxAttachmentsPerMessage = 8
	maxAttachmentJSONBytes   = 64 << 10
	maxAttachmentName        = 255
	maxAttachmentMIME        = 128
	maxAttachmentRef         = 1024
	maxAttachmentSize        = 50 << 20
)

// Attachment is display-only metadata for one user message. File bytes stay
// in the shell/app; the session snapshot carries only this metadata.
type Attachment struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	MIME     string `json:"mime,omitempty"`
	Size     int64  `json:"size,omitempty"`
	LocalRef string `json:"local_ref,omitempty"`
}

type messageAttachments struct {
	MessageIndex int          `json:"message_index"`
	Role         string       `json:"role"`
	Attachments  []Attachment `json:"attachments"`
}

// parseAttachments validates the JSON array supplied by the shell. It rejects
// oversized lists and fields before anything enters message history.
func parseAttachments(raw string) ([]Attachment, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" {
		return nil, nil
	}
	if len(raw) > maxAttachmentJSONBytes {
		return nil, fmt.Errorf("attachments JSON exceeds %d bytes", maxAttachmentJSONBytes)
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	var items []Attachment
	if err := dec.Decode(&items); err != nil {
		return nil, fmt.Errorf("invalid attachments JSON: %w", err)
	}
	if len(items) > maxAttachmentsPerMessage {
		return nil, fmt.Errorf("too many attachments: %d (max %d)", len(items), maxAttachmentsPerMessage)
	}
	for i := range items {
		items[i].Name = strings.TrimSpace(items[i].Name)
		items[i].MIME = strings.TrimSpace(items[i].MIME)
		items[i].LocalRef = strings.TrimSpace(items[i].LocalRef)
		if items[i].ID == "" {
			items[i].ID = fmt.Sprintf("att_%d", i+1)
		}
	}
	if err := validateAttachments(items); err != nil {
		return nil, err
	}
	return items, nil
}

func validateAttachments(items []Attachment) error {
	for i, a := range items {
		if a.Name == "" {
			return fmt.Errorf("attachment %d: name is required", i)
		}
		if len(a.Name) > maxAttachmentName {
			return fmt.Errorf("attachment %d: name exceeds %d bytes", i, maxAttachmentName)
		}
		if len(a.MIME) > maxAttachmentMIME {
			return fmt.Errorf("attachment %d: mime exceeds %d bytes", i, maxAttachmentMIME)
		}
		if a.Size < 0 || a.Size > maxAttachmentSize {
			return fmt.Errorf("attachment %d: size must be between 0 and %d", i, maxAttachmentSize)
		}
		if a.LocalRef == "" {
			return fmt.Errorf("attachment %d: local_ref is required", i)
		}
		if len(a.LocalRef) > maxAttachmentRef {
			return fmt.Errorf("attachment %d: local_ref exceeds %d bytes", i, maxAttachmentRef)
		}
	}
	return nil
}

// SessionAttachments returns attachment metadata grouped by message index for
// the current session. The shell uses it to render cards after ImportSession.
func (a *Agent) SessionAttachments() string {
	a.mu.Lock()
	out := make([]messageAttachments, 0)
	for i, m := range a.msgs {
		if len(m.Attachments) == 0 {
			continue
		}
		out = append(out, messageAttachments{
			MessageIndex: i,
			Role:         m.Role,
			Attachments:  append([]Attachment(nil), m.Attachments...),
		})
	}
	a.mu.Unlock()
	b, _ := json.Marshal(map[string]any{"attachments": out})
	return string(b)
}

// SessionAttachments on the default session.
func SessionAttachments() string { return defaultSession.SessionAttachments() }
