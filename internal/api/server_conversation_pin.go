package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
)

const pinnedConversationsKey = "pinned_conversations"
const maxPinnedConversations = 200

func (s *Server) pinnedConversationIDs() map[string]bool {
	out := map[string]bool{}
	if s.Store == nil {
		return out
	}
	raw, ok, err := s.Store.GetSetting(pinnedConversationsKey)
	if err != nil || !ok {
		return out
	}
	var ids []string
	if json.Unmarshal(raw, &ids) != nil {
		return out
	}
	for _, id := range ids {
		if id != "" {
			out[id] = true
		}
	}
	return out
}

// handleSetConversationPin pins or unpins one conversation. Pins are stored in
// settings so they survive restarts without a schema migration.
func (s *Server) handleSetConversationPin(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	var body struct {
		Pinned bool `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	var ids []string
	if raw, ok, err := s.Store.GetSetting(pinnedConversationsKey); err == nil && ok {
		_ = json.Unmarshal(raw, &ids)
	}
	next := make([]string, 0, len(ids)+1)
	found := false
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if id == convID {
			found = true
			if !body.Pinned {
				continue
			}
		}
		next = append(next, id)
	}
	if body.Pinned && !found {
		if len(next) >= maxPinnedConversations {
			writeError(w, http.StatusBadRequest, "invalid_request", "too many pinned conversations")
			return
		}
		next = append(next, convID)
	}
	raw, _ := json.Marshal(next)
	if err := s.Store.UpsertSetting(pinnedConversationsKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": convID,
		"pinned":          body.Pinned,
	})
}

// applySummaryPins marks pinned conversations and moves them to the front while
// preserving the existing within-group order.
func (s *Server) applySummaryPins(out []conversation.Summary) []conversation.Summary {
	pins := s.pinnedConversationIDs()
	if len(pins) == 0 || len(out) == 0 {
		return out
	}
	pinned := make([]conversation.Summary, 0, len(out))
	rest := make([]conversation.Summary, 0, len(out))
	for _, sum := range out {
		if pins[sum.ID] {
			sum.Pinned = true
			pinned = append(pinned, sum)
			continue
		}
		rest = append(rest, sum)
	}
	return append(pinned, rest...)
}
