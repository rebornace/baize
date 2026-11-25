package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/conversation"
)

// handleMoveConversationWorkspace moves one conversation to another workspace.
// Active runs are rejected so events and approvals cannot straddle workspaces.
func (s *Server) handleMoveConversationWorkspace(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	var body struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
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
	if s.Store != nil {
		if busy, err := s.Store.HasActiveRun(convID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		} else if busy {
			writeError(w, http.StatusConflict, "conversation_busy",
				"conversation has an active run; finish it before moving")
			return
		}
	}
	ms := s.metaStore()
	if ms == nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "conversation meta store not configured")
		return
	}
	if err := ms.UpdateMetaWorkspace(convID, wsID); err != nil {
		if errors.Is(err, conversation.ErrMetaNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"conversation_id": convID,
		"workspace_id":    wsID,
	})
}
