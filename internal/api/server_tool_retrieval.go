package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/toolretrieval"
)

func (s *Server) handleGetToolRetrieval(w http.ResponseWriter, r *http.Request) {
	if s.ToolRetrieval == nil {
		writeJSON(w, http.StatusOK, toolretrieval.Status{
			Mode:      "standard",
			Phase:     toolretrieval.PhaseDisabled,
			Provider:  toolretrieval.ProviderLocal,
			Installer: toolretrieval.InstallerForGOOS(),
			Model:     toolretrieval.DefaultEmbedModel,
			Paths:     toolretrieval.DiscoverPaths(""),
		})
		return
	}
	writeJSON(w, http.StatusOK, s.ToolRetrieval.Snapshot(r.Context()))
}

func (s *Server) handlePostToolRetrievalEnable(w http.ResponseWriter, r *http.Request) {
	if s.ToolRetrieval == nil {
		writeError(w, http.StatusServiceUnavailable, "tool_retrieval_unavailable", "tool retrieval manager not wired")
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_body", "cannot read body")
		return
	}
	trimmed := strings.TrimSpace(string(body))
	// Empty body / {} ⇒ local Ollama one-click install (async).
	if trimmed == "" || trimmed == "{}" {
		if err := s.ToolRetrieval.StartEnable(context.Background()); err != nil {
			writeError(w, http.StatusConflict, "tool_retrieval_busy", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, s.ToolRetrieval.Snapshot(r.Context()))
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
	if prov == "" || prov == toolretrieval.ProviderLocal {
		if err := s.ToolRetrieval.StartEnable(context.Background()); err != nil {
			writeError(w, http.StatusConflict, "tool_retrieval_busy", err.Error())
			return
		}
		writeJSON(w, http.StatusAccepted, s.ToolRetrieval.Snapshot(r.Context()))
		return
	}
	if prov != toolretrieval.ProviderAPI {
		writeError(w, http.StatusBadRequest, "invalid_provider", "provider must be local or api")
		return
	}
	if strings.TrimSpace(req.BaseURL) == "" {
		writeError(w, http.StatusBadRequest, "base_url_required", "base_url is required for api provider")
		return
	}
	if err := s.ToolRetrieval.EnableAPI(r.Context(), toolretrieval.EnableAPIRequest{
		BaseURL: req.BaseURL,
		Model:   req.Model,
		APIKey:  req.APIKey,
	}); err != nil {
		writeJSON(w, http.StatusOK, s.ToolRetrieval.Snapshot(r.Context()))
		return
	}
	writeJSON(w, http.StatusOK, s.ToolRetrieval.Snapshot(r.Context()))
}

func (s *Server) handlePostToolRetrievalDisable(w http.ResponseWriter, r *http.Request) {
	if s.ToolRetrieval == nil {
		writeError(w, http.StatusServiceUnavailable, "tool_retrieval_unavailable", "tool retrieval manager not wired")
		return
	}
	if err := s.ToolRetrieval.Disable(); err != nil {
		writeError(w, http.StatusInternalServerError, "tool_retrieval_disable_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.ToolRetrieval.Snapshot(r.Context()))
}

func (s *Server) handlePostToolRetrievalCleanup(w http.ResponseWriter, r *http.Request) {
	if s.ToolRetrieval == nil {
		writeError(w, http.StatusServiceUnavailable, "tool_retrieval_unavailable", "tool retrieval manager not wired")
		return
	}
	var opts toolretrieval.CleanupOptions
	body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if strings.TrimSpace(string(body)) != "" {
		_ = json.Unmarshal(body, &opts)
	}
	rep, err := s.ToolRetrieval.Cleanup(r.Context(), opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "tool_retrieval_cleanup_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"cleanup": rep,
		"status":  s.ToolRetrieval.Snapshot(r.Context()),
	})
}

func (s *Server) handlePutToolRetrievalPaths(w http.ResponseWriter, r *http.Request) {
	if s.ToolRetrieval == nil {
		writeError(w, http.StatusServiceUnavailable, "tool_retrieval_unavailable", "tool retrieval manager not wired")
		return
	}
	var req struct {
		ModelsDir *string `json:"models_dir"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", "invalid JSON body")
		return
	}
	if req.ModelsDir == nil {
		writeError(w, http.StatusBadRequest, "models_dir_required", "models_dir is required (use empty string to clear)")
		return
	}
	if err := s.ToolRetrieval.SetModelsDir(r.Context(), *req.ModelsDir); err != nil {
		writeError(w, http.StatusBadRequest, "models_dir_invalid", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.ToolRetrieval.Snapshot(r.Context()))
}
