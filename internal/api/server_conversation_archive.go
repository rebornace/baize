package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
)

// handleSetConversationArchived archives or restores a conversation. Archived
// conversations stay readable and searchable; only the default sidebar list
// hides them. New activity auto-unarchives them (see prepareConversationMeta).
func (s *Server) handleSetConversationArchived(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	var body struct {
		Archived bool `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	ms := s.metaStore()
	if ms == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "conversation meta store not configured")
		return
	}
	if err := ms.UpdateMetaArchived(convID, body.Archived); err != nil {
		if errors.Is(err, conversation.ErrMetaNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": convID,
		"archived":        body.Archived,
	})
}
