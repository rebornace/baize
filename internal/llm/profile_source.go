package llm

import (
	"fmt"

	"github.com/rebornace/baize/internal/store"
)

// StoreProfileSource adapts store.Store to the llm.ProfileSource interface.
type StoreProfileSource struct {
	Store ModelProfileStore
}

// ModelProfileStore is the subset of store.Store used by the source.
type ModelProfileStore interface {
	ListModelProfiles() ([]store.ModelProfile, error)
	GetModelProfile(id string) (store.ModelProfile, error)
}

func toView(p store.ModelProfile) ModelProfileView {
	return ModelProfileView{
		ID:              p.ID,
		Provider:        p.Provider,
		BaseURL:         p.BaseURL,
		Model:           p.Model,
		APIKey:          p.APIKey,
		APIKeyEnv:       p.APIKeyEnv,
		DisableThinking: p.DisableThinking,
		SupportsVision:  p.SupportsVision,
		UpdatedAt:       p.UpdatedAt,
	}
}

// DefaultModelProfile returns the profile flagged IsDefault. When none is
// flagged it falls back to the first profile (stores order by creation time);
// an empty store is an error because no model is configured.
func (s *StoreProfileSource) DefaultModelProfile() (ModelProfileView, error) {
	list, err := s.Store.ListModelProfiles()
	if err != nil {
		return ModelProfileView{}, err
	}
	for _, p := range list {
		if p.IsDefault {
			return toView(p), nil
		}
	}
	if len(list) > 0 {
		return toView(list[0]), nil
	}
	return ModelProfileView{}, fmt.Errorf("no model profile configured")
}

// ModelProfileByID resolves a single profile. An empty id yields the store's
// not-found error (no profile has an empty id), so callers never silently
// resolve to a real profile when no id was supplied.
func (s *StoreProfileSource) ModelProfileByID(id string) (ModelProfileView, error) {
	p, err := s.Store.GetModelProfile(id)
	if err != nil {
		return ModelProfileView{}, err
	}
	return toView(p), nil
}
