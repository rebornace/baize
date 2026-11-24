package api

import (
	"context"
	"strings"

	"github.com/rebornace/baize/internal/llm"
)

// hydrateRunUserParts rebuilds the original user turn's image parts from the
// persisted conversation message and the channel-media store. It is used when a
// live job payload no longer carries multimodal content (reconcile / HITL).
func (s *Server) hydrateRunUserParts(ctx context.Context, runID, convID, input string) []llm.ContentPart {
	if s.Messages == nil || s.ChannelMedia == nil || convID == "" || runID == "" {
		return nil
	}
	var content string
	for _, m := range s.Messages.List(convID) {
		if m.RunID == runID && strings.EqualFold(m.Role, "user") {
			content = m.Content
			break
		}
	}
	if content == "" {
		return nil
	}
	parts := []llm.ContentPart{{Type: "text", Text: input}}
	for _, match := range attachmentImageRef.FindAllStringSubmatch(content, -1) {
		object := mediaObjectFromURL(match[2])
		if object == "" {
			continue
		}
		data, mime, found, err := s.ChannelMedia.OpenMedia(ctx, convID, object)
		if err != nil || !found || !strings.HasPrefix(mime, "image/") {
			continue
		}
		parts = append(parts, llm.ContentPart{Type: "image", ImageMIME: mime, ImageBytes: data})
	}
	if len(parts) == 1 {
		return nil
	}
	return parts
}

// HydrateRunUserParts is the exported form used by bootstrap wiring.
func (s *Server) HydrateRunUserParts(ctx context.Context, runID, convID, input string) []llm.ContentPart {
	return s.hydrateRunUserParts(ctx, runID, convID, input)
}
