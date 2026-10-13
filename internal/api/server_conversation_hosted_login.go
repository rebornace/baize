package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/hostedlogin"
)

// POST /v0/conversations/{id}/hosted-login — issue a conversation-scoped hosted
// login session and return its public URL (Chat「浏览器登录」).
func (s *Server) handlePostConversationHostedLogin(w http.ResponseWriter, r *http.Request) {
	convID := strings.TrimSpace(r.PathValue("id"))
	if convID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing conversation id")
		return
	}
	if s.HostedSessions == nil {
		writeError(w, http.StatusServiceUnavailable, "unavailable", "hosted login unavailable")
		return
	}
	origin := strings.TrimRight(strings.TrimSpace(s.publicBaseURL()), "/")
	if origin == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "public_base_url is required")
		return
	}
	var body struct {
		ConnectorID string `json:"connector_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid json body")
		return
	}
	connectorID := strings.TrimSpace(body.ConnectorID)
	if connectorID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "connector_id is required")
		return
	}
	conn, err := s.Store.GetConnector(connectorID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "unknown connector_id")
		return
	}
	recipe := conn.Auth.HostedLogin
	if !recipe.Enabled || strings.TrimSpace(recipe.LoginTool) == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "hosted login is not enabled for this connector")
		return
	}
	if err := hostedlogin.ValidateRecipe(recipe); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	id, _, err := s.HostedSessions.Issue(hostedlogin.Session{
		ConnectorID:    connectorID,
		LoginTool:      recipe.LoginTool,
		Mode:           recipe.Mode,
		ConversationID: convID,
		Fields:         recipe.Fields,
		PublicOrigin:   origin,
	})
	if err != nil || id == "" {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to issue hosted login session")
		return
	}
	s.hostedAudit().Record(hostedlogin.AuditEvent{
		Kind:           hostedlogin.AuditSessionIssue,
		SessionID:      id,
		ConnectorID:    connectorID,
		ConversationID: convID,
		ClientIP:       clientIP(r),
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"session_id": id,
		"url":        origin + "/v0/auth/hosted/" + id,
	})
}
