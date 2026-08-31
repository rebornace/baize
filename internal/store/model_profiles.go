package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrModelProfileNotFound is returned when a model profile row is missing.
var ErrModelProfileNotFound = errors.New("model profile not found")

// RedactAPIKey masks a secret for API responses: keeps first 3 and last 4
// characters. Empty input returns "". Callers must never persist the redacted
// form back as the real key.
func RedactAPIKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return ""
	}
	if len(k) <= 8 {
		return "••••"
	}
	return k[:3] + "…" + k[len(k)-4:]
}

// IsRedactedAPIKey reports whether s looks like a redacted placeholder echoed
// back from the UI (contains the ellipsis) and therefore must not overwrite a
// stored key.
func IsRedactedAPIKey(s string) bool {
	return strings.Contains(s, "…") || strings.Contains(s, "•")
}

func (s *Memory) UpsertModelProfile(p ModelProfile) (ModelProfile, error) {
	if strings.TrimSpace(p.Name) == "" {
		return ModelProfile{}, fmt.Errorf("model profile name is required")
	}
	if strings.TrimSpace(p.Provider) == "" {
		p.Provider = "openai_compatible"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if p.ID == "" {
		p.ID = "mp_" + uuid.NewString()
		p.CreatedAt = now
		for _, ex := range s.modelProfiles {
			if ex.Name == p.Name {
				return ModelProfile{}, fmt.Errorf("model profile name %q already exists", p.Name)
			}
		}
	} else {
		ex, ok := s.modelProfiles[p.ID]
		if !ok {
			return ModelProfile{}, ErrModelProfileNotFound
		}
		if p.APIKey == "" || IsRedactedAPIKey(p.APIKey) {
			p.APIKey = ex.APIKey
		}
		p.CreatedAt = ex.CreatedAt
		for _, other := range s.modelProfiles {
			if other.ID != p.ID && other.Name == p.Name {
				return ModelProfile{}, fmt.Errorf("model profile name %q already exists", p.Name)
			}
		}
		p.IsDefault = ex.IsDefault // default only changes via SetDefaultModelProfile
	}
	p.UpdatedAt = now
	s.modelProfiles[p.ID] = p
	return p, nil
}

func (s *Memory) GetModelProfile(id string) (ModelProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	p, ok := s.modelProfiles[id]
	if !ok {
		return ModelProfile{}, ErrModelProfileNotFound
	}
	return p, nil
}

func (s *Memory) ListModelProfiles() ([]ModelProfile, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]ModelProfile, 0, len(s.modelProfiles))
	for _, p := range s.modelProfiles {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *Memory) DeleteModelProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.modelProfiles[id]
	if !ok {
		return ErrModelProfileNotFound
	}
	if p.IsDefault {
		return fmt.Errorf("cannot delete the default model profile; set another as default first")
	}
	delete(s.modelProfiles, id)
	return nil
}

func (s *Memory) SetDefaultModelProfile(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.modelProfiles[id]; !ok {
		return ErrModelProfileNotFound
	}
	for k, p := range s.modelProfiles {
		p.IsDefault = (k == id)
		s.modelProfiles[k] = p
	}
	return nil
}
