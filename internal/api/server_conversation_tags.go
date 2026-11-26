package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
)

const maxConversationTags = 8
const maxConversationTagRunes = 24

// normalizeConversationTags trims, dedupes case-insensitively and caps tags.
func normalizeConversationTags(raw []string) []string {
	out := make([]string, 0, len(raw))
	seen := map[string]bool{}
	for _, item := range raw {
		tag := strings.TrimSpace(item)
		if tag == "" {
			continue
		}
		if len([]rune(tag)) > maxConversationTagRunes {
			continue
		}
		key := strings.ToLower(tag)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, tag)
		if len(out) >= maxConversationTags {
			break
		}
	}
	return out
}

// handleSetConversationTags replaces a conversation's tag list.
func (s *Server) handleSetConversationTags(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	var body struct {
		Tags []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	for _, raw := range body.Tags {
		if len([]rune(strings.TrimSpace(raw))) > maxConversationTagRunes {
			writeError(w, http.StatusBadRequest, "invalid_request",
				"each tag must be at most 24 characters")
			return
		}
	}
	tags := normalizeConversationTags(body.Tags)
	ms := s.metaStore()
	if ms == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "conversation meta store not configured")
		return
	}
	if err := ms.UpdateMetaTags(convID, tags); err != nil {
		if errors.Is(err, conversation.ErrMetaNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversation_id": convID, "tags": tags})
}
