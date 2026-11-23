package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/rebornace/baize/internal/llm"
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
	ThinkingLevel   *string `json:"thinking_level"`
	ThinkingDialect *string `json:"thinking_dialect"`
	SupportsVision  *bool   `json:"supports_vision"`
	ContextTokens   *int    `json:"context_tokens"`
	// AutoTier is "light" | "standard" | "power" | "auto". "auto" (or empty on
	// create) infers the tier from the model name server-side.
	AutoTier *string `json:"auto_tier"`
}

func redactedProfile(p store.ModelProfile) store.ModelProfile {
	p.APIKey = store.RedactAPIKey(p.APIKey)
	return p
}

func redactMCPForAPI(cfg store.MCPConfig) store.MCPConfig {
	if cfg.OAuth == nil {
		return cfg
	}
	o := *cfg.OAuth
	o.TokenBundleSealed = ""
	o.ClientSecretSealed = ""
	cfg.OAuth = &o
	return cfg
}

func mergeMCPOAuthPreserveSecrets(mcp *store.MCPConfig, existing store.Connector, hasExisting bool) {
	if mcp == nil || mcp.OAuth == nil || !hasExisting || existing.MCP.OAuth == nil {
		return
	}
	if strings.TrimSpace(mcp.OAuth.ClientSecretSealed) == "" {
		mcp.OAuth.ClientSecretSealed = existing.MCP.OAuth.ClientSecretSealed
	}
	if strings.TrimSpace(mcp.OAuth.TokenBundleSealed) == "" {
		mcp.OAuth.TokenBundleSealed = existing.MCP.OAuth.TokenBundleSealed
	}
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
	if writeIfSettingsKeyRequired(w, err) {
		return
	}
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

// resolveTier computes the persisted tier from a payload value. "auto" or an
// empty string means infer from the model/profile name; explicit light/standard
// /power are honored; anything else is rejected. On create a nil value infers.
func resolveTier(raw *string, modelName, profileName string) (string, error) {
	v := ""
	if raw != nil {
		v = strings.ToLower(strings.TrimSpace(*raw))
	}
	switch v {
	case "", "auto":
		return llm.InferTier(firstNonEmptyStr(modelName, profileName)), nil
	case store.AutoTierLight, store.AutoTierStandard, store.AutoTierPower:
		return v, nil
	default:
		return "", errors.New("invalid auto_tier: want light, standard, power, or auto")
	}
}

func firstNonEmptyStr(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
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
	tier, err := resolveTier(p.AutoTier, strVal(p.Model), strVal(p.Name))
	if err != nil {
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
		AutoTier:        tier,
	}
	applyThinkingPayload(&prof, p, true)
	if p.ContextTokens != nil && *p.ContextTokens > 0 {
		prof.ContextTokens = *p.ContextTokens
	}
	saved, err := s.Store.UpsertModelProfile(prof)
	if err != nil {
		writeUpsertError(w, err)
		return
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
	applyThinkingPayload(&updated, p, false)
	if p.SupportsVision != nil {
		updated.SupportsVision = *p.SupportsVision
	}
	if p.ContextTokens != nil && *p.ContextTokens > 0 {
		updated.ContextTokens = *p.ContextTokens
	}
	if p.AutoTier != nil {
		tier, err := resolveTier(p.AutoTier, updated.Model, updated.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid_profile", err.Error())
			return
		}
		updated.AutoTier = tier
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
		default:
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		}
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
	return validateThinkingPayload(p)
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
	return validateThinkingPayload(p)
}

func validateThinkingPayload(p modelProfilePayload) error {
	if p.ThinkingLevel != nil {
		lvl := strings.ToLower(strings.TrimSpace(*p.ThinkingLevel))
		if lvl != "" && !validThinkingLevel(lvl) {
			return errors.New("invalid thinking_level")
		}
	}
	if p.ThinkingDialect != nil {
		d := strings.ToLower(strings.TrimSpace(*p.ThinkingDialect))
		if d != "" && !validThinkingDialect(d) {
			return errors.New("invalid thinking_dialect")
		}
	}
	return nil
}

func validThinkingLevel(lvl string) bool {
	switch lvl {
	case store.ThinkingOff, store.ThinkingLow, store.ThinkingMedium, store.ThinkingHigh:
		return true
	default:
		return false
	}
}

func validThinkingDialect(d string) bool {
	switch d {
	case store.ThinkingDialectAuto, store.ThinkingDialectOpenAI, store.ThinkingDialectDeepSeek,
		store.ThinkingDialectQwen, store.ThinkingDialectOmit:
		return true
	default:
		return false
	}
}

// applyThinkingPayload merges thinking fields onto a profile.
// On create (isCreate), omitted level falls through to DisableThinking via Sync.
// On patch: thinking_level wins when both are present; only disable_thinking
// clears ThinkingLevel so Sync remaps from the flag.
func applyThinkingPayload(prof *store.ModelProfile, p modelProfilePayload, isCreate bool) {
	if p.ThinkingDialect != nil {
		prof.ThinkingDialect = strings.ToLower(strings.TrimSpace(*p.ThinkingDialect))
	}
	if p.ThinkingLevel != nil {
		lvl := strings.ToLower(strings.TrimSpace(*p.ThinkingLevel))
		prof.ThinkingLevel = lvl
		return
	}
	if isCreate {
		// Leave ThinkingLevel empty so SyncProfileThinking maps from DisableThinking.
		return
	}
	if p.DisableThinking != nil {
		// Only the legacy flag changed: clear level so Sync remaps.
		prof.ThinkingLevel = ""
	}
}
