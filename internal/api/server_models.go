package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/store"
)

type modelProfilePayload struct {
	Name            string `json:"name"`
	Provider        string `json:"provider"`
	BaseURL         string `json:"base_url"`
	Model           string `json:"model"`
	APIKey          string `json:"api_key"`
	APIKeyEnv       string `json:"api_key_env"`
	DisableThinking bool   `json:"disable_thinking"`
	SupportsVision  bool   `json:"supports_vision"`
	IsDefault       bool   `json:"is_default"`
}

func redactedProfile(p store.ModelProfile) store.ModelProfile {
	p.APIKey = store.RedactAPIKey(p.APIKey)
	return p
}

func (s *Server) handleListModelProfiles(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListModelProfiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list_failed", err.Error())
		return
	}
	out := make([]store.ModelProfile, 0, len(list))
	for _, p := range list {
		out = append(out, redactedProfile(p))
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": out})
}

func (s *Server) handlePostModelProfile(w http.ResponseWriter, r *http.Request) {
	var p modelProfilePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := validateModelPayload(p, false); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	prof := store.ModelProfile{
		Name:            strings.TrimSpace(p.Name),
		Provider:        "openai_compatible",
		BaseURL:         strings.TrimSpace(p.BaseURL),
		Model:           strings.TrimSpace(p.Model),
		APIKey:          p.APIKey,
		APIKeyEnv:       strings.TrimSpace(p.APIKeyEnv),
		DisableThinking: p.DisableThinking,
		SupportsVision:  p.SupportsVision,
	}
	saved, err := s.Store.UpsertModelProfile(prof)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upsert_failed", err.Error())
		return
	}
	if p.IsDefault {
		if err := s.Store.SetDefaultModelProfile(saved.ID); err != nil {
			writeError(w, http.StatusBadRequest, "set_default_failed", err.Error())
			return
		}
		saved.IsDefault = true
	}
	writeJSON(w, http.StatusCreated, map[string]any{"profile": redactedProfile(saved)})
}

func (s *Server) handlePatchModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, err := s.Store.GetModelProfile(id); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "model profile not found")
		return
	}
	var p modelProfilePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := validateModelPayload(p, true); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	updated := store.ModelProfile{
		ID:              id,
		Name:            strings.TrimSpace(p.Name),
		Provider:        "openai_compatible",
		BaseURL:         strings.TrimSpace(p.BaseURL),
		Model:           strings.TrimSpace(p.Model),
		APIKey:          p.APIKey, // empty/redacted -> store keeps existing
		APIKeyEnv:       strings.TrimSpace(p.APIKeyEnv),
		DisableThinking: p.DisableThinking,
		SupportsVision:  p.SupportsVision,
	}
	saved, err := s.Store.UpsertModelProfile(updated)
	if err != nil {
		writeError(w, http.StatusBadRequest, "upsert_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": redactedProfile(saved)})
}

func (s *Server) handleDeleteModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteModelProfile(id); err != nil {
		writeError(w, http.StatusBadRequest, "delete_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSetDefaultModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.SetDefaultModelProfile(id); err != nil {
		writeError(w, http.StatusBadRequest, "set_default_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func validateModelPayload(p modelProfilePayload, isPatch bool) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(p.BaseURL) == "" {
		return fmt.Errorf("base_url is required")
	}
	if strings.TrimSpace(p.Model) == "" {
		return fmt.Errorf("model is required")
	}
	// On create, at least one credential source must be present. On patch,
	// an empty/redacted key means "keep existing", so do not force it.
	if !isPatch && strings.TrimSpace(p.APIKey) == "" && strings.TrimSpace(p.APIKeyEnv) == "" {
		return fmt.Errorf("either api_key or api_key_env is required")
	}
	return nil
}
