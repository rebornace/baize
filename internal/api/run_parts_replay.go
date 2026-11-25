package api

import (
	"context"
	"strings"

	"github.com/rebornace/baize/internal/attach"
	"github.com/rebornace/baize/internal/llm"
)

// attachmentTextBlock formats one extracted text attachment for the model.
// It is shared by the first turn and HITL/reconcile replay so resumed runs see
// the same block shape as the original run.
func attachmentTextBlock(filename, text string) string {
	return "\n\n【附件: " + filename + "】\n" + attach.UntrustedContentNotice + "\n" + text
}

// hydrateRunUserParts rebuilds the original user turn's text and image parts
// from the persisted conversation message and the channel-media store. It is
// used when a live job payload no longer carries multimodal content
// (reconcile / HITL).
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
	text := input
	var images []llm.ContentPart
	var files []llm.ContentPart
	supportsPDF := s.runSupportsPDF(runID)
	appendAttachment := func(filename string, data []byte, mime string) {
		opts := attach.DefaultOptions()
		opts.MaxTextChars = 0
		texts, imgs, err := attach.ProcessDecoded([]attach.Decoded{{
			Filename: filename, MediaType: mime, Data: data,
		}}, opts)
		if err != nil {
			return
		}
		for _, t := range texts {
			if t.Kind == "text" {
				text += attachmentTextBlock(filename, t.Text)
			}
		}
		for _, img := range imgs {
			if img.Kind == "image" {
				images = append(images, llm.ContentPart{
					Type: "image", ImageMIME: img.ImageMIME, ImageBytes: img.ImageBytes,
				})
			}
		}
	}
	for _, match := range attachmentImageRef.FindAllStringSubmatch(content, -1) {
		object := mediaObjectFromURL(match[2])
		if object == "" {
			continue
		}
		data, mime, found, err := s.ChannelMedia.OpenMedia(ctx, convID, object)
		if err != nil || !found || !strings.HasPrefix(mime, "image/") {
			continue
		}
		images = append(images, llm.ContentPart{Type: "image", ImageMIME: mime, ImageBytes: data})
	}
	for _, match := range attachmentFileRef.FindAllStringSubmatch(content, -1) {
		filename := strings.TrimSpace(match[1])
		object := mediaObjectFromURL(match[2])
		if object == "" || filename == "" {
			continue
		}
		data, mime, found, err := s.ChannelMedia.OpenMedia(ctx, convID, object)
		if err != nil || !found {
			continue
		}
		if strings.HasPrefix(mime, "image/") {
			images = append(images, llm.ContentPart{Type: "image", ImageMIME: mime, ImageBytes: data})
			continue
		}
		if mime == "application/pdf" && supportsPDF {
			files = append(files, llm.ContentPart{
				Type: "file", FileMIME: mime, FileBytes: data, Filename: filename,
			})
			continue
		}
		appendAttachment(filename, data, mime)
	}
	if text == input && len(images) == 0 && len(files) == 0 {
		return nil
	}
	parts := []llm.ContentPart{{Type: "text", Text: text}}
	parts = append(parts, images...)
	return append(parts, files...)
}

// HydrateRunUserParts is the exported form used by bootstrap wiring.
func (s *Server) HydrateRunUserParts(ctx context.Context, runID, convID, input string) []llm.ContentPart {
	return s.hydrateRunUserParts(ctx, runID, convID, input)
}

func (s *Server) runSupportsPDF(runID string) bool {
	if s.Store == nil || runID == "" {
		return false
	}
	rec, err := s.Store.GetRun(runID)
	if err != nil || rec == nil || rec.ModelProfileID == "" {
		return false
	}
	p, err := s.Store.GetModelProfile(rec.ModelProfileID)
	return err == nil && p.SupportsPDF
}
