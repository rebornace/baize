package api

import (
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

// POST /v0/connectors/{id}/hosted-login/candidates — rule-inferred login methods.
func (s *Server) handlePostConnectorHostedLoginCandidates(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing connector id")
		return
	}
	conn, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	if conn.Type != "openapi" && conn.Type != "http" {
		writeError(w, http.StatusBadRequest, "invalid_request", "hosted login candidates are only for openapi/http connectors")
		return
	}

	glob := strings.TrimSpace(conn.Auth.Capture.ToolNameGlob)
	if glob == "" {
		glob = "*login*"
	}
	schemes, err := connectorSecuritySchemes(conn)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "could not read connector spec security schemes")
		return
	}
	cands := hostedlogin.BuildCandidates(s.Store.ListToolsByConnector(id), glob, schemes)
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
}

func connectorSecuritySchemes(conn store.Connector) (map[string]any, error) {
	specPath := strings.TrimSpace(conn.Spec)
	if specPath == "" {
		return map[string]any{}, nil
	}
	return openapi.LoadSecuritySchemes(specPath)
}
