package api

import (
	"context"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const attachmentMediaPrefix = "/v0/channels/media/"

var (
	attachmentImageRef = regexp.MustCompile(`!\[([^\]]*)\]\((/v0/channels/media/[^)\s]+)\)`)
	attachmentFileRef  = regexp.MustCompile(`\[file:([^\]]+)\]\((/v0/channels/media/[^)\s]+)\)`)
)

// backfillAttachmentsFromMessages indexes attachments referenced by existing
// chat bubbles. Uploads made after 0.42 are indexed at save time; this covers
// older web and channel attachments the first time a conversation's attachment
// list is opened. Runs once per conversation per process.
func (s *Server) backfillAttachmentsFromMessages(ctx context.Context, convID string) {
	if s.Messages == nil || convID == "" {
		return
	}
	if _, loaded := s.attachmentBackfilled.LoadOrStore(convID, true); loaded {
		return
	}
	msgs := s.Messages.List(convID)
	for _, m := range msgs {
		for _, match := range attachmentImageRef.FindAllStringSubmatch(m.Content, -1) {
			alt, rawURL := match[1], match[2]
			object := mediaObjectFromURL(rawURL)
			if object == "" {
				continue
			}
			name := mediaNameFromURL(rawURL)
			if name == "" && alt != "" && alt != "图片" {
				name = alt
			}
			if name == "" {
				name = object
			}
			s.indexBackfilledAttachment(ctx, convID, object, name, m.CreatedAt)
		}
		for _, match := range attachmentFileRef.FindAllStringSubmatch(m.Content, -1) {
			name, rawURL := match[1], match[2]
			object := mediaObjectFromURL(rawURL)
			if object == "" {
				continue
			}
			if name == "" {
				name = object
			}
			s.indexBackfilledAttachment(ctx, convID, object, name, m.CreatedAt)
		}
	}
}

func (s *Server) indexBackfilledAttachment(ctx context.Context, convID, object, filename string, createdAt time.Time) {
	if _, ok, err := s.Store.GetConversationAttachment(convID, object); err != nil || ok {
		return
	}
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	}
	_ = s.Store.AddConversationAttachment(store.ConversationAttachment{
		ConversationID: convID,
		Object:         object,
		Filename:       filename,
		MediaType:      mimeForAttachment(object),
		CreatedAt:      createdAt,
	})
}

func mediaObjectFromURL(rawURL string) string {
	if !strings.HasPrefix(rawURL, attachmentMediaPrefix) {
		return ""
	}
	trimmed := rawURL
	if i := strings.IndexByte(trimmed, '?'); i >= 0 {
		trimmed = trimmed[:i]
	}
	if i := strings.LastIndexByte(trimmed, '/'); i >= 0 {
		trimmed = trimmed[i+1:]
	}
	if trimmed == "" || strings.Contains(trimmed, "..") {
		return ""
	}
	return trimmed
}

func mediaNameFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(parsed.Query().Get("name"))
}

func mimeForAttachment(object string) string {
	switch strings.ToLower(path.Ext(object)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".pdf":
		return "application/pdf"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".txt":
		return "text/plain"
	case ".md":
		return "text/markdown"
	case ".csv":
		return "text/csv"
	}
	return ""
}
