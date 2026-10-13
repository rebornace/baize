package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

// POST /v0/connectors/{id}/hosted-login/draft — infer a disabled HostedLoginRecipe
// from login tools (preferred) or OpenAPI security schemes.
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
	var draft store.HostedLoginRecipe
	for _, tl := range s.Store.ListToolsByConnector(id) {
		if !matchToolGlob(glob, tl.Name) {
			continue
		}
		draft = hostedlogin.DraftRecipe(tl.Name, tl.InputSchema)
		break
	}
	if draft.Mode == "" {
		// Fallback: token/basic/header from nothing — leave empty mode for UI.
		draft = store.HostedLoginRecipe{Enabled: false}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"hosted_login": draft,
		"source":       draftSource(draft),
	})
}

func draftSource(d store.HostedLoginRecipe) string {
	if d.LoginTool != "" {
		return "login_tool"
	}
	if d.Mode != "" {
		return "inferred"
	}
	return "empty"
}

func matchToolGlob(glob, name string) bool {
	glob = strings.TrimSpace(glob)
	name = strings.TrimSpace(name)
	if glob == "" || glob == "*" {
		return true
	}
	if strings.EqualFold(glob, "__none__") {
		return false
	}
	ok, err := filepath.Match(glob, name)
	return err == nil && ok
}
