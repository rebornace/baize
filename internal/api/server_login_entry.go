package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/loginentry"
)

func (s *Server) handleListLoginEntries(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}
	filter := strings.TrimSpace(r.URL.Query().Get("connector_id"))
	entries := loginentry.List(s.Store, s.Identities, convID, filter)
	if entries == nil {
		entries = []loginentry.Entry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": entries})
}

func (s *Server) handleLoginInvoke(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if !s.requireConversationAccess(w, r, convID) {
		return
	}

	var body struct {
		AgentID     string         `json:"agent_id"`
		ConnectorID string         `json:"connector_id"`
		ToolName    string         `json:"tool_name"`
		Arguments   map[string]any `json:"arguments"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	if strings.TrimSpace(body.AgentID) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "agent_id is required")
		return
	}
	if _, err := s.Store.GetAgent(body.AgentID); err != nil {
		writeError(w, http.StatusNotFound, "agent_not_found", "unknown agent")
		return
	}

	entry, ok := loginentry.Find(s.Store, s.Identities, convID, strings.TrimSpace(body.ConnectorID), strings.TrimSpace(body.ToolName))
	if !ok {
		writeError(w, http.StatusBadRequest, "not_a_login_entry", "tool is not a login entry for this conversation")
		return
	}
	args := body.Arguments
	if args == nil {
		args = map[string]any{}
	}
	if err := loginentry.ValidateArgs(entry.Required, args); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}

	busy, err := s.Store.HasActiveRun(convID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if busy {
		writeError(w, http.StatusConflict, "conversation_busy", "conversation has an active run")
		return
	}

	bubble := fmt.Sprintf("已发起登录 · %s", entry.Title)
	updated, err := s.startRun(r.Context(), startRunInput{
		AgentID:        body.AgentID,
		Input:          bubble,
		ConversationID: convID,
		BubbleContent:  bubble,
		ForcedToolName: entry.ToolName,
		ForcedToolArgs: args,
	})
	if err != nil {
		if err.Error() == "无权访问该会话" {
			writeError(w, http.StatusForbidden, "forbidden", err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"run_id":          updated.ID,
		"status":          updated.Status,
		"conversation_id": convID,
	})
}
