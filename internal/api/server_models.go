package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/store"
)

const supportedModelProvider = "openai_compatible"

// modelProfilePayload uses pointer fields so PATCH can distinguish "field
// omitted" (nil -> keep existing) from "field set to zero/false/empty".
type modelProfilePayload struct {
	Name            *string `json:"name"`
	Provider        *string `json:"provider"`
	BaseURL         *string `json:"base_url"`
	Model           *string `json:"model"`
	APIKey          *string `json:"api_key"`
	APIKeyEnv       *string `json:"api_key_env"`
	DisableThinking *bool   `json:"disable_thinking"`
	SupportsVision  *bool   `json:"supports_vision"`
	ContextTokens   *int    `json:"context_tokens"`
	IsDefault       bool    `json:"is_default"`
}

func redactedProfile(p store.ModelProfile) store.ModelProfile {
	p.APIKey = store.RedactAPIKey(p.APIKey)
	return p
}

func strVal(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

// validateProvider rejects any explicit provider this version cannot serve.
func validateProvider(p *string) error {
	if p == nil {
		return nil
	}
	v := strings.TrimSpace(*p)
	if v == "" || v == supportedModelProvider {
		return nil
	}
	return errors.New("unsupported provider: only openai_compatible is supported")
}

// upsertStatus maps store upsert errors to HTTP statuses: validation/conflict
// are client errors (400); anything else is an internal failure (500).
func writeUpsertError(w http.ResponseWriter, err error) {
	msg := err.Error()
	if strings.Contains(msg, "already exists") || strings.Contains(msg, "name is required") {
		writeError(w, http.StatusBadRequest, "upsert_failed", msg)
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", msg)
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
	if err := validateModelCreate(p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}
	prof := store.ModelProfile{
		Name:            strVal(p.Name),
		Provider:        supportedModelProvider,
		BaseURL:         strVal(p.BaseURL),
		Model:           strVal(p.Model),
		APIKey:          strVal(p.APIKey),
		APIKeyEnv:       strVal(p.APIKeyEnv),
		DisableThinking: p.DisableThinking != nil && *p.DisableThinking,
		SupportsVision:  p.SupportsVision != nil && *p.SupportsVision,
	}
	if p.ContextTokens != nil && *p.ContextTokens > 0 {
		prof.ContextTokens = *p.ContextTokens
	}
	saved, err := s.Store.UpsertModelProfile(prof)
	if err != nil {
		writeUpsertError(w, err)
		return
	}
	if p.IsDefault {
		if err := s.Store.SetDefaultModelProfile(saved.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "set_default_failed", err.Error())
			return
		}
		saved.IsDefault = true
	}
	writeJSON(w, http.StatusCreated, map[string]any{"profile": redactedProfile(saved)})
}

func (s *Server) handlePatchModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	existing, err := s.Store.GetModelProfile(id)
	if err != nil {
		if errors.Is(err, store.ErrModelProfileNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "model profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	var p modelProfilePayload
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if err := validateModelPatch(p); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_profile", err.Error())
		return
	}

	// Field-level merge: start from the stored row and overwrite only the
	// fields present in the payload.
	updated := existing
	if p.Name != nil {
		updated.Name = strVal(p.Name)
	}
	if p.BaseURL != nil {
		updated.BaseURL = strVal(p.BaseURL)
	}
	if p.Model != nil {
		updated.Model = strVal(p.Model)
	}
	if p.APIKey != nil {
		// Empty/redacted value is preserved by the store layer; a real value
		// overwrites. nil (omitted) keeps the stored key.
		updated.APIKey = *p.APIKey
	}
	if p.APIKeyEnv != nil {
		// Empty string is an explicit clear; nil keeps the stored value.
		updated.APIKeyEnv = strings.TrimSpace(*p.APIKeyEnv)
	}
	if p.DisableThinking != nil {
		updated.DisableThinking = *p.DisableThinking
	}
	if p.SupportsVision != nil {
		updated.SupportsVision = *p.SupportsVision
	}
	if p.ContextTokens != nil && *p.ContextTokens > 0 {
		updated.ContextTokens = *p.ContextTokens
	}
	updated.Provider = supportedModelProvider

	saved, err := s.Store.UpsertModelProfile(updated)
	if err != nil {
		writeUpsertError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"profile": redactedProfile(saved)})
}

func (s *Server) handleDeleteModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.DeleteModelProfile(id); err != nil {
		switch {
		case errors.Is(err, store.ErrModelProfileNotFound):
			writeError(w, http.StatusNotFound, "not_found", "model profile not found")
		case strings.Contains(err.Error(), "cannot delete"):
			writeError(w, http.StatusBadRequest, "delete_failed", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleSetDefaultModelProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := s.Store.SetDefaultModelProfile(id); err != nil {
		if errors.Is(err, store.ErrModelProfileNotFound) {
			writeError(w, http.StatusNotFound, "not_found", "model profile not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "set_default_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func validateModelCreate(p modelProfilePayload) error {
	if err := validateProvider(p.Provider); err != nil {
		return err
	}
	if strVal(p.Name) == "" {
		return errors.New("name is required")
	}
	if strVal(p.BaseURL) == "" {
		return errors.New("base_url is required")
	}
	if strVal(p.Model) == "" {
		return errors.New("model is required")
	}
	// A new profile must carry at least one credential source.
	if strVal(p.APIKey) == "" && strVal(p.APIKeyEnv) == "" {
		return errors.New("either api_key or api_key_env is required")
	}
	return nil
}

func validateModelPatch(p modelProfilePayload) error {
	if err := validateProvider(p.Provider); err != nil {
		return err
	}
	if p.Name != nil && strVal(p.Name) == "" {
		return errors.New("name must not be empty")
	}
	if p.BaseURL != nil && strVal(p.BaseURL) == "" {
		return errors.New("base_url must not be empty")
	}
	if p.Model != nil && strVal(p.Model) == "" {
		return errors.New("model must not be empty")
	}
	return nil
}
