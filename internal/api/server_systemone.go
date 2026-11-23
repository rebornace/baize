package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/systemoneenable"
)

func (s *Server) handleGetSystemOne(w http.ResponseWriter, r *http.Request) {
	if s.SystemOneEnable == nil {
		writeJSON(w, http.StatusOK, (*systemoneenable.Manager)(nil).Snapshot(r.Context()))
		return
	}
	writeJSON(w, http.StatusOK, s.SystemOneEnable.Snapshot(r.Context()))
}

func (s *Server) handlePostSystemOneEnable(w http.ResponseWriter, r *http.Request) {
	if s.SystemOneEnable == nil {
		writeError(w, http.StatusServiceUnavailable, "systemone_unavailable", "systemone manager not wired")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "cannot read body")
		return
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" || trimmed == "{}" {
		if err := s.SystemOneEnable.StartEnable(context.Background()); err != nil {
			writeError(w, http.StatusConflict, "systemone_busy", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, s.SystemOneEnable.Snapshot(r.Context()))
		return
	}

	var req struct {
		Provider string `json:"provider"`
		BaseURL  string `json:"base_url"`
		Model    string `json:"model"`
		APIKey   string `json:"api_key"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}
	prov := strings.TrimSpace(req.Provider)
	if prov == "" || prov == systemoneenable.ProviderLocal {
		if err := s.SystemOneEnable.StartEnable(context.Background()); err != nil {
			writeError(w, http.StatusConflict, "systemone_busy", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, s.SystemOneEnable.Snapshot(r.Context()))
		return
	}
	if prov != systemoneenable.ProviderAPI {
		writeError(w, http.StatusBadRequest, "invalid_provider", "provider must be local or api")
		return
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		writeError(w, http.StatusBadRequest, "base_url_required", "base_url is required for api provider")
		return
	}
	if err := s.SystemOneEnable.EnableAPI(r.Context(), systemoneenable.EnableAPIRequest{
		BaseURL: req.BaseURL,
		Model:   req.Model,
		APIKey:  req.APIKey,
	}); err != nil {
		writeJSON(w, http.StatusOK, s.SystemOneEnable.Snapshot(r.Context()))
		return
	}
	writeJSON(w, http.StatusOK, s.SystemOneEnable.Snapshot(r.Context()))
}

func (s *Server) handlePostSystemOneDisable(w http.ResponseWriter, r *http.Request) {
	if s.SystemOneEnable == nil {
		writeError(w, http.StatusServiceUnavailable, "systemone_unavailable", "systemone manager not wired")
		return
	}
	if err := s.SystemOneEnable.Disable(); err != nil {
		writeError(w, http.StatusInternalServerError, "systemone_disable_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.SystemOneEnable.Snapshot(r.Context()))
}

func (s *Server) handlePostSystemOneCleanup(w http.ResponseWriter, r *http.Request) {
	if s.SystemOneEnable == nil {
		writeError(w, http.StatusServiceUnavailable, "systemone_unavailable", "systemone manager not wired")
		return
	}
	rep, err := s.SystemOneEnable.Cleanup(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "systemone_cleanup_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cleanup": rep,
		"status":  s.SystemOneEnable.Snapshot(r.Context()),
	})
}
