package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

const batchConversationLimit = 200

type batchSkipJSON struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

func (s *Server) batchConversationIDs(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var body struct {
		ConversationIDs []string `json:"conversation_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return nil, false
	}
	if len(body.ConversationIDs) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "conversation_ids is required")
		return nil, false
	}
	if len(body.ConversationIDs) > batchConversationLimit {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"at most 200 conversations per request")
		return nil, false
	}
	ids := make([]string, 0, len(body.ConversationIDs))
	for _, raw := range body.ConversationIDs {
		if id := strings.TrimSpace(raw); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "conversation_ids is empty")
		return nil, false
	}
	return ids, true
}

func (s *Server) handleBatchMoveConversations(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationIDs []string `json:"conversation_ids"`
		WorkspaceID     string   `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if len(body.ConversationIDs) == 0 || len(body.ConversationIDs) > batchConversationLimit {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"conversation_ids must contain 1-200 ids")
		return
	}
	wsID := conversation.NormalizeWorkspaceID(body.WorkspaceID)
	if wst := s.workspaceStore(); wst != nil {
		if _, err := wst.GetWorkspace(wsID); err != nil {
			writeError(w, http.StatusNotFound, "not_found", "workspace not found")
			return
		}
	} else if wsID != conversation.DefaultWorkspaceID {
		writeError(w, http.StatusNotFound, "not_found", "workspace not found")
		return
	}
	ms := s.metaStore()
	if ms == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "conversation meta store not configured")
		return
	}
	moved := 0
	skipped := make([]batchSkipJSON, 0)
	for _, raw := range body.ConversationIDs {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if err := s.checkConversationAccess(r.Context(), id); err != nil {
			reason := "error"
			if errors.Is(err, errConversationForbidden) {
				reason = "forbidden"
			}
			skipped = append(skipped, batchSkipJSON{ID: id, Reason: reason})
			continue
		}
		if s.Store != nil {
			busy, err := s.Store.HasActiveRun(id)
			if err != nil {
				skipped = append(skipped, batchSkipJSON{ID: id, Reason: "error"})
				continue
			}
			if busy {
				skipped = append(skipped, batchSkipJSON{ID: id, Reason: "conversation_busy"})
				continue
			}
		}
		if err := ms.UpdateMetaWorkspace(id, wsID); err != nil {
			reason := "error"
			if errors.Is(err, conversation.ErrMetaNotFound) {
				reason = "not_found"
			}
			skipped = append(skipped, batchSkipJSON{ID: id, Reason: reason})
			continue
		}
		moved++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"moved":        moved,
		"skipped":      skipped,
		"workspace_id": wsID,
	})
}

func (s *Server) handleBatchArchiveConversations(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ConversationIDs []string `json:"conversation_ids"`
		Archived        *bool    `json:"archived"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.Archived == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "archived is required")
		return
	}
	if len(body.ConversationIDs) == 0 || len(body.ConversationIDs) > batchConversationLimit {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"conversation_ids must contain 1-200 ids")
		return
	}
	ms := s.metaStore()
	if ms == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "conversation meta store not configured")
		return
	}
	updated := 0
	skipped := make([]batchSkipJSON, 0)
	seen := map[string]bool{}
	for _, raw := range body.ConversationIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		if err := s.checkConversationAccess(r.Context(), id); err != nil {
			reason := "error"
			if errors.Is(err, errConversationForbidden) {
				reason = "forbidden"
			}
			skipped = append(skipped, batchSkipJSON{ID: id, Reason: reason})
			continue
		}
		if err := ms.UpdateMetaArchived(id, *body.Archived); err != nil {
			reason := "error"
			if errors.Is(err, conversation.ErrMetaNotFound) {
				reason = "not_found"
			}
			skipped = append(skipped, batchSkipJSON{ID: id, Reason: reason})
			continue
		}
		updated++
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"updated":  updated,
		"archived": *body.Archived,
		"skipped":  skipped,
	})
}

func (s *Server) handleBatchDeleteConversations(w http.ResponseWriter, r *http.Request) {
	ids, ok := s.batchConversationIDs(w, r)
	if !ok {
		return
	}
	deleted := 0
	skipped := make([]batchSkipJSON, 0)
	for _, id := range ids {
		if err := s.checkConversationAccess(r.Context(), id); err != nil {
			reason := "error"
			if errors.Is(err, errConversationForbidden) {
				reason = "forbidden"
			}
			skipped = append(skipped, batchSkipJSON{ID: id, Reason: reason})
			continue
		}
		if err := s.deleteConversationData(id); err != nil {
			reason := "error"
			switch {
			case errors.Is(err, store.ErrConversationBusy):
				reason = "conversation_busy"
			case errors.Is(err, conversation.ErrMetaNotFound):
				reason = "not_found"
			}
			skipped = append(skipped, batchSkipJSON{ID: id, Reason: reason})
			continue
		}
		deleted++
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": deleted, "skipped": skipped})
}
