package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
)

const maxConversationTitleRunes = 100

// handleRenameConversation sets a user-facing title. An empty title clears the
// override and falls back to the first user message.
func (s *Server) handleRenameConversation(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	var body struct {
		Title string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	title := strings.TrimSpace(body.Title)
	if len([]rune(title)) > maxConversationTitleRunes {
		writeError(w, http.StatusBadRequest, "invalid_request", "title is too long")
		return
	}
	ms := s.metaStore()
	if ms == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "conversation meta store not configured")
		return
	}
	if err := ms.UpdateMetaTitle(convID, title); err != nil {
		if errors.Is(err, conversation.ErrMetaNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": convID,
		"title":           title,
	})
}
