package api

import (
	"errors"
	"net/http"
)

// handleChannelMedia serves an inbound channel image (e.g. a WeChat photo)
// for inline display in the web UI. The conversation id is in the path so the
// existing conversation ACL applies: only an admin or the owning operator may
// fetch it. Bytes are streamed (the frontend fetches with auth headers and
// turns them into an object URL, the same pattern as artifact pages), so no
// token is placed in the URL.
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
	data, mime, found, err := s.ChannelMedia.OpenImage(r.Context(), convID, object)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "open media failed")
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "media_not_found", "image not found")
		return
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
