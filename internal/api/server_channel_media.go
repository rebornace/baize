package api

import (
	"errors"
	"net/http"
	"strings"
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
