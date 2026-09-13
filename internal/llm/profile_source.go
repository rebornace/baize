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
		ContextTokens:   p.ContextTokens,
		Tier:            store.NormalizeAutoTier(p.AutoTier),
		UpdatedAt:       p.UpdatedAt,
	}
}

// ListProfiles returns all profiles ordered by creation time (the store
// already sorts that way).
func (s *StoreProfileSource) ListProfiles() ([]ModelProfileView, error) {
	list, err := s.Store.ListModelProfiles()
	if err != nil {
		return nil, err
	}
	out := make([]ModelProfileView, 0, len(list))
	for _, p := range list {
		out = append(out, toView(p))
	}
	return out, nil
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

// PrimaryModelProfile picks the model used when Auto routing returns no
// explicit id and when background tasks (compaction) need a generic model:
// prefer the earliest-created standard-tier profile, then the earliest profile
// overall. An empty list is an error because no model is configured.
func PrimaryModelProfile(list []ModelProfileView) (ModelProfileView, error) {
	for _, v := range list {
		if v.Tier == store.AutoTierStandard {
			return v, nil
		}
	}
	if len(list) > 0 {
		return list[0], nil
	}
	return ModelProfileView{}, fmt.Errorf("no model profile configured")
}
