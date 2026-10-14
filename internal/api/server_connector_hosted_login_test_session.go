package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

// POST /v0/connectors/{id}/hosted-login/test-session — issue an export-scoped
// hosted login session for admin testing (External Login wizard).
func (s *Server) handlePostConnectorHostedLoginTestSession(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing connector id")
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
		ExportKeyID string `json:"export_key_id"`
		MethodID    string `json:"method_id"`
	}
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	conn, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	cfg := conn.Auth.HostedLogin
	if !cfg.Enabled {
		writeError(w, http.StatusBadRequest, "invalid_request", "hosted login is not enabled for this connector")
		return
	}
	methodID := strings.TrimSpace(body.MethodID)
	if methodID == "" {
		methodID = strings.TrimSpace(cfg.DefaultMethodID)
	}
	if methodID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "hosted login has no default method configured")
		return
	}
	role := ""
	if m, ok := cfg.MethodByID(methodID); ok {
		role = m.EffectiveRole()
	}

	key, identityID, ok := s.resolveHostedTestExportAuth(id, strings.TrimSpace(body.ExportKeyID), role)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_request", "bind an MCP export key with an identity slot for this connector")
		return
	}

	seed, err := hostedlogin.MaterializeSession(cfg, methodID, hostedlogin.Session{
		ExportKeyID:  key.ID,
		ConnectorID:  id,
		IdentityID:   identityID,
		PublicOrigin: origin,
	})
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	sessID, _, err := s.HostedSessions.Issue(seed)
	if err != nil || sessID == "" {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to issue hosted login session")
		return
	}
	s.hostedAudit().Record(hostedlogin.AuditEvent{
		Kind:        hostedlogin.AuditSessionIssue,
		SessionID:   sessID,
		ConnectorID: id,
		ClientIP:    clientIP(r),
	})
	writeJSON(w, http.StatusCreated, map[string]any{
		"session_id": sessID,
		"url":        origin + "/v0/auth/hosted/" + sessID,
	})
}

func (s *Server) resolveHostedTestExportAuth(connectorID, exportKeyID, role string) (store.MCPExportKey, string, bool) {
	if exportKeyID != "" {
		key, err := s.Store.GetMCPExportKey(exportKeyID)
		if err != nil {
			return store.MCPExportKey{}, "", false
		}
		if key.RevokedAt != nil {
			return store.MCPExportKey{}, "", false
		}
		identityID, ok := store.ResolveExportIdentityID(key, connectorID, role)
		if !ok || strings.TrimSpace(identityID) == "" {
			return store.MCPExportKey{}, "", false
		}
		return key, identityID, true
	}
	keys, err := s.Store.ListMCPExportKeys("")
	if err != nil {
		return store.MCPExportKey{}, "", false
	}
	for _, key := range keys {
		if key.RevokedAt != nil {
			continue
		}
		identityID, ok := store.ResolveExportIdentityID(key, connectorID, role)
		if !ok || strings.TrimSpace(identityID) == "" {
			continue
		}
		return key, identityID, true
	}
	return store.MCPExportKey{}, "", false
}
