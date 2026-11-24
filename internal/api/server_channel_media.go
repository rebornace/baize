package api

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// handleChannelMedia serves an inbound channel attachment (a WeChat photo, or a
// file such as a .docx/.pdf/.zip) for inline display or download in the web UI.
// The conversation id is in the path so the existing conversation ACL applies:
// only an admin or the owning operator may fetch it. The frontend fetches with
// auth headers (a plain <img>/<a> cannot send the Bearer token) and turns the
// bytes into an object URL, so no token is placed in the URL. Images are served
// inline; any other type is served as an attachment download.
func (s *Server) handleChannelMedia(w http.ResponseWriter, r *http.Request) {
	if s.ChannelMedia == nil {
		writeError(w, http.StatusNotFound, "media_not_found", "channel media is not configured")
		return
	}
	convID := r.PathValue("conv")
	object := r.PathValue("object")
	if convID == "" || object == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation or object")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	data, mime, found, err := s.ChannelMedia.OpenMedia(r.Context(), convID, object)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "open media failed")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "media_not_found", "attachment not found")
		return
	}
	// object is "<uuid><ext>"; use its base name as a safe download fallback.
	safeName := object
	if i := strings.LastIndexByte(object, '/'); i >= 0 {
		safeName = object[i+1:]
	}
	w.Header().Set("Cache-Control", "private, max-age=300")
	if strings.HasPrefix(mime, "image/") {
		w.Header().Set("Content-Type", mime)
	} else {
		w.Header().Set("Content-Type", mime)
		w.Header().Set("Content-Disposition", "attachment; filename=\""+safeName+"\"")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// deleteConversationAttachment removes one upload and its derived workspace
// copy. The index is preferred for the exact workspace path; `fallbackName` is
// only used for attachments saved before the index existed.
func (s *Server) deleteConversationAttachment(ctx context.Context, convID, object, fallbackName string) error {
	rec, indexed, _ := s.Store.GetConversationAttachment(convID, object)
	if err := s.ChannelMedia.DeleteMedia(ctx, convID, object); err != nil {
		return err
	}
	if s.Workspace != nil {
		switch {
		case indexed && rec.WorkspacePath != "":
			if fd, ok := s.Workspace.(interface {
				DeleteFile(context.Context, string, string) error
			}); ok {
				if err := fd.DeleteFile(ctx, convID, rec.WorkspacePath); err != nil {
					log.Printf("workspace: delete %q: %v", rec.WorkspacePath, err)
				}
			}
		case !indexed && fallbackName != "":
			if err := s.Workspace.DeleteUpload(ctx, convID, fallbackName); err != nil {
				log.Printf("workspace: delete upload %q: %v", fallbackName, err)
			}
		}
	}
	if indexed {
		_ = s.Store.DeleteConversationAttachment(convID, object)
	}
	return nil
}

// handleDeleteChannelMedia removes a persisted attachment (the original upload
// used for preview/download) and its indexed workspace copy. Conversation ACL
// is the same as the GET route.
func (s *Server) handleDeleteChannelMedia(w http.ResponseWriter, r *http.Request) {
	if s.ChannelMedia == nil {
		writeError(w, http.StatusNotFound, "media_not_found", "channel media is not configured")
		return
	}
	convID := r.PathValue("conv")
	object := r.PathValue("object")
	if convID == "" || object == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation or object")
		return
	}
	if strings.ContainsAny(object, "/\\") || strings.Contains(object, "..") || object == "." {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid object name")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if err := s.deleteConversationAttachment(r.Context(), convID, object, name); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "delete media failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleListConversationAttachments lists the indexed uploads of a conversation
// so the UI can show an attachment overview and delete in bulk.
func (s *Server) handleListConversationAttachments(w http.ResponseWriter, r *http.Request) {
	convID := r.PathValue("id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	limit := 100
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n <= 0 || n > 500 {
			writeError(w, http.StatusBadRequest, "invalid_request", "limit must be between 1 and 500")
			return
		}
		limit = n
	}
	s.backfillAttachmentsFromMessages(r.Context(), convID)
	list, err := s.Store.ListConversationAttachments(convID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	items := make([]map[string]any, 0, len(list))
	for _, a := range list {
		items = append(items, map[string]any{
			"object":     a.Object,
			"filename":   a.Filename,
			"media_type": a.MediaType,
			"size_bytes": a.SizeBytes,
			"created_at": a.CreatedAt.UTC().Format(time.RFC3339),
			"url":        "/v0/channels/media/" + convID + "/" + a.Object,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

// handleBulkDeleteConversationAttachments deletes up to 200 indexed uploads.
// Missing objects are ignored so retries are safe.
func (s *Server) handleBulkDeleteConversationAttachments(w http.ResponseWriter, r *http.Request) {
	if s.ChannelMedia == nil {
		writeError(w, http.StatusNotFound, "media_not_found", "channel media is not configured")
		return
	}
	convID := r.PathValue("id")
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if err := s.checkConversationAccess(r.Context(), convID); err != nil {
		if errors.Is(err, errConversationForbidden) {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var body struct {
		Objects []string `json:"objects"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if len(body.Objects) == 0 || len(body.Objects) > 200 {
		writeError(w, http.StatusBadRequest, "invalid_request", "objects must contain between 1 and 200 items")
		return
	}
	deleted, failed := 0, 0
	for _, raw := range body.Objects {
		object := strings.TrimSpace(raw)
		if object == "" || object == "." || strings.ContainsAny(object, "/\\") || strings.Contains(object, "..") {
			failed++
			continue
		}
		if err := s.deleteConversationAttachment(r.Context(), convID, object, ""); err != nil {
			log.Printf("chat media: bulk delete %q: %v", object, err)
			failed++
			continue
		}
		deleted++
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "failed": failed})
}
