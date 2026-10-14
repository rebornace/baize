package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

type hostedLoginSuggestRequest struct {
	Candidates []store.HostedLoginMethod `json:"candidates,omitempty"`
}

// POST /v0/connectors/{id}/hosted-login/suggest — AI label hints for rule candidates (no store write).
func (s *Server) handlePostConnectorHostedLoginSuggest(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(r.PathValue("id"))
	if id == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "missing connector id")
		return
	}
	if s.LLM == nil {
		writeError(w, http.StatusBadGateway, "llm_unavailable", "LLM is not configured")
		return
	}

	conn, err := s.Store.GetConnector(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "connector_not_found", "connector not found")
		return
	}
	if conn.Type != "openapi" && conn.Type != "http" {
		writeError(w, http.StatusBadRequest, "invalid_request", "hosted login suggest is only for openapi/http connectors")
		return
	}

	var req hostedLoginSuggestRequest
	if r.Body != nil {
		dec := json.NewDecoder(r.Body)
		if err := dec.Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
			return
		}
	}

	cands := req.Candidates
	if len(cands) == 0 {
		glob := strings.TrimSpace(conn.Auth.Capture.ToolNameGlob)
		if glob == "" {
			glob = "*login*"
		}
		schemes, _ := s.connectorSecuritySchemes(r.Context(), conn)
		cands = hostedlogin.BuildCandidates(s.Store.ListToolsByConnector(id), glob, schemes)
	}

	suggestions, err := hostedlogin.SuggestLoginLabels(r.Context(), s.LLM, cands)
	if err != nil {
		writeError(w, http.StatusBadGateway, "suggest_failed", err.Error())
		return
	}
	if suggestions == nil {
		suggestions = []hostedlogin.LoginMethodSuggestion{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"suggestions": suggestions})
}
