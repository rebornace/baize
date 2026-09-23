package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
)

// discoverModelsRequest lists models at an OpenAI-compatible endpoint without
// persisting anything. On create the key comes straight from the form; on edit
// the client may omit api_key and pass the profile id so the stored (sealed)
// key is used server-side and never sent to the browser.
type discoverModelsRequest struct {
	BaseURL   string `json:"base_url"`
	APIKey    string `json:"api_key"`
	APIKeyEnv string `json:"api_key_env"`
	ProfileID string `json:"profile_id"`
}

// batchImportModelsRequest creates one profile per selected model, all sharing
// the same base_url + credential. Each item may carry an optional display name;
// when empty the model id is used as the name.
type batchImportModelsRequest struct {
	BaseURL   string             `json:"base_url"`
	APIKey    string             `json:"api_key"`
	APIKeyEnv string             `json:"api_key_env"`
	Models    []batchModelChoice `json:"models"`
	// ProfileID, when set and no explicit api_key is given, reuses the stored
	// (sealed) key for that profile — used when batch-importing from an edit.
	ProfileID string `json:"profile_id"`
	// Optional defaults applied to every created profile.
	ThinkingLevel   string `json:"thinking_level"`
	ThinkingDialect string `json:"thinking_dialect"`
	SupportsVision  *bool  `json:"supports_vision"`
	ContextTokens   int    `json:"context_tokens"`
}

type batchModelChoice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// batchImportResult reports what happened per chosen model so the UI can show
// created vs skipped rows without guessing.
type batchImportResult struct {
	Created []store.ModelProfile `json:"created"`
	Skipped []batchSkipped       `json:"skipped"`
}

type batchSkipped struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// resolveDiscoverCredential returns the plaintext api key for a discovery
// request: an explicit form key wins; otherwise, when profile_id is supplied,
// the stored key is decrypted; finally an env-var name may be resolved.
func (s *Server) resolveDiscoverCredential(req discoverModelsRequest) (string, error) {
	if k := strings.TrimSpace(req.APIKey); k != "" && !store.IsRedactedAPIKey(k) {
		return k, nil
	}
	if id := strings.TrimSpace(req.ProfileID); id != "" {
		existing, err := s.Store.GetModelProfile(id)
		if err != nil {
			if errors.Is(err, store.ErrModelProfileNotFound) {
				return "", fmt.Errorf("model profile not found")
			}
			return "", err
		}
		if k := strings.TrimSpace(existing.APIKey); k != "" {
			return k, nil
		}
	}
	if env := strings.TrimSpace(req.APIKeyEnv); env != "" {
		return strings.TrimSpace(os.Getenv(env)), nil
	}
	// Some endpoints (e.g. local servers) require no key.
	return "", nil
}

func (s *Server) handleDiscoverModels(w http.ResponseWriter, r *http.Request) {
	var req discoverModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "base_url is required")
		return
	}
	key, err := s.resolveDiscoverCredential(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "discover_failed", err.Error())
		return
	}

	models, err := llm.FetchModels(r.Context(), baseURL, key)
	if err != nil {
		writeError(w, http.StatusBadGateway, "discover_failed", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"models": models})
}

func (s *Server) handleBatchImportModels(w http.ResponseWriter, r *http.Request) {
	var req batchImportModelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	baseURL := strings.TrimSpace(req.BaseURL)
	if baseURL == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "base_url is required")
		return
	}
	if len(req.Models) == 0 {
		writeError(w, http.StatusBadRequest, "invalid_request", "select at least one model")
		return
	}

	apiKey := strings.TrimSpace(req.APIKey)
	if apiKey != "" && store.IsRedactedAPIKey(apiKey) {
		apiKey = ""
	}
	apiKeyEnv := strings.TrimSpace(req.APIKeyEnv)
	if apiKey == "" && apiKeyEnv == "" {
		// Edit flow: no key typed in the form, so reuse the credential of the
		// profile being edited rather than failing.
		if id := strings.TrimSpace(req.ProfileID); id != "" {
			existing, err := s.Store.GetModelProfile(id)
			if err == nil {
				if k := strings.TrimSpace(existing.APIKey); k != "" {
					apiKey = k
				}
				if env := strings.TrimSpace(existing.APIKeyEnv); env != "" && apiKey == "" {
					apiKeyEnv = env
				}
			}
		}
	}
	if apiKey == "" && apiKeyEnv == "" {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"either api_key or api_key_env is required")
		return
	}

	result := batchImportResult{Created: []store.ModelProfile{}, Skipped: []batchSkipped{}}
	for _, choice := range req.Models {
		id := strings.TrimSpace(choice.ID)
		if id == "" {
			continue
		}
		name := strings.TrimSpace(choice.Name)
		if name == "" {
			name = id
		}

		prof := store.ModelProfile{
			Name:            name,
			Provider:        supportedModelProvider,
			BaseURL:         baseURL,
			Model:           id,
			APIKey:          apiKey,
			APIKeyEnv:       apiKeyEnv,
			AutoTier:        llm.InferTier(id),
			SupportsVision:  req.SupportsVision != nil && *req.SupportsVision,
			ContextTokens:   req.ContextTokens,
			ThinkingLevel:   normalizeThinkingLevel(req.ThinkingLevel),
			ThinkingDialect: normalizeThinkingDialect(req.ThinkingDialect),
		}

		saved, err := s.Store.UpsertModelProfile(prof)
		if err != nil {
			reason := err.Error()
			if strings.Contains(reason, "already exists") {
				result.Skipped = append(result.Skipped, batchSkipped{
					ID: id, Name: name, Reason: "name already exists",
				})
				continue
			}
			result.Skipped = append(result.Skipped, batchSkipped{
				ID: id, Name: name, Reason: reason,
			})
			continue
		}
		result.Created = append(result.Created, redactedProfile(saved))
	}

	writeJSON(w, http.StatusOK, map[string]any{"result": result})
}

func normalizeThinkingLevel(lvl string) string {
	lvl = strings.ToLower(strings.TrimSpace(lvl))
	if lvl == "" {
		return store.ThinkingMedium
	}
	if validThinkingLevel(lvl) {
		return lvl
	}
	return store.ThinkingMedium
}

func normalizeThinkingDialect(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	if d == "" {
		return store.ThinkingDialectAuto
	}
	if validThinkingDialect(d) {
		return d
	}
	return store.ThinkingDialectAuto
}
