package api

import (
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

// POST /v0/connectors/{id}/hosted-login/draft — infer a disabled hosted login
// config from login tools (preferred) or OpenAPI security schemes.
func (s *Server) handlePostConnectorHostedLoginDraft(w http.ResponseWriter, r *http.Request) {
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
		writeError(w, http.StatusBadRequest, "invalid_request", "hosted login draft is only for openapi/http connectors")
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
	draft := draftConfigFromCandidates(cands)
	writeJSON(w, http.StatusOK, map[string]any{
		"hosted_login": draft,
		"source":       draftSource(draft),
	})
}

func draftSource(c store.HostedLoginConfig) string {
	m, ok := c.DefaultMethod()
	if !ok {
		return "empty"
	}
	if m.LoginTool != "" {
		return "login_tool"
	}
	if m.Mode != "" {
		return "inferred"
	}
	return "empty"
}

func draftConfigFromCandidates(cands []store.HostedLoginMethod) store.HostedLoginConfig {
	if len(cands) == 0 {
		return store.HostedLoginConfig{Enabled: false}
	}
	const draftID = "draft"
	m := cands[0]
	m.ID = draftID
	return store.HostedLoginConfig{
		Enabled:         false,
		DefaultMethodID: draftID,
		Methods:         []store.HostedLoginMethod{m},
	}
}
