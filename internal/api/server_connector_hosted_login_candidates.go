package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/connector/specstore"
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
	// Schemes are optional: login-tool candidates still work when the spec
	// cannot be re-read (legacy path / missing blob). Prefer empty over hard fail.
	schemes, _ := s.connectorSecuritySchemes(r.Context(), conn)
	cands := hostedlogin.BuildCandidates(s.Store.ListToolsByConnector(id), glob, schemes)
	writeJSON(w, http.StatusOK, map[string]any{"candidates": cands})
}

func (s *Server) connectorSecuritySchemes(ctx context.Context, conn store.Connector) (map[string]any, error) {
	spec := strings.TrimSpace(conn.Spec)
	if spec == "" {
		return map[string]any{}, nil
	}
	data, err := s.readConnectorSpecBytes(ctx, spec)
	if err != nil {
		return nil, err
	}
	return openapi.LoadSecuritySchemesFromBytes(data)
}

func (s *Server) readConnectorSpecBytes(ctx context.Context, spec string) ([]byte, error) {
	if specstore.IsBlobSpecKey(spec) {
		if s.Blobs == nil {
			return nil, fmt.Errorf("blob store is not configured")
		}
		return s.Blobs.Get(ctx, spec)
	}
	if specstore.IsLegacyConnectorFSPath(spec) {
		return nil, fmt.Errorf("filesystem spec path is no longer supported")
	}
	return os.ReadFile(spec)
}
