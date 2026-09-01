package llm

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// ModelProfileView is the subset of store.ModelProfile the Switch needs.
// Defined here to avoid llm depending on the store package.
type ModelProfileView struct {
	ID              string
	Provider        string
	BaseURL         string
	Model           string
	APIKey          string
	APIKeyEnv       string
	DisableThinking bool
	SupportsVision  bool
	ContextTokens   int
	UpdatedAt       time.Time
}

// ProfileSource resolves model profiles (backed by store.Store in production).
type ProfileSource interface {
	DefaultModelProfile() (ModelProfileView, error)
	ModelProfileByID(id string) (ModelProfileView, error)
}

type ctxKey int

const modelProfileIDKey ctxKey = iota

// WithModelProfileID attaches a per-run model profile choice to the context.
func WithModelProfileID(ctx context.Context, profileID string) context.Context {
	if profileID == "" {
		return ctx
	}
	return context.WithValue(ctx, modelProfileIDKey, profileID)
}

// ModelProfileIDFromContext returns the per-run profile id, or "".
func ModelProfileIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(modelProfileIDKey).(string); ok {
		return v
	}
	return ""
}

type cachedProvider struct {
	prov      Provider
	updatedAt time.Time
}

// Switch is a Provider that resolves the active model per-run from a
// ProfileSource. Provider instances are cached by profile id and rebuilt when
// the profile's UpdatedAt advances (hot reload, no restart).
type Switch struct {
	src   ProfileSource
	mu    sync.Mutex
	cache map[string]*cachedProvider

	// build constructs a Provider from a profile. Overridable in tests.
	build func(ModelProfileView) Provider
}

func NewSwitch(src ProfileSource) *Switch {
	s := &Switch{src: src, cache: map[string]*cachedProvider{}}
	s.build = s.defaultBuild
	return s
}

func (s *Switch) defaultBuild(v ModelProfileView) Provider {
	key := v.APIKey
	if key == "" && v.APIKeyEnv != "" {
		key = os.Getenv(v.APIKeyEnv)
	}
	p := NewOpenAI(v.BaseURL, key, v.Model)
	p.DisableThinking = v.DisableThinking
	p.VisionSupported = v.SupportsVision
	return p
}

func (s *Switch) providerFor(ctx context.Context) (Provider, error) {
	id := ModelProfileIDFromContext(ctx)
	view, err := s.src.ModelProfileByID(id)
	if err != nil || id == "" || view.ID == "" {
		view, err = s.src.DefaultModelProfile()
		if err != nil {
			return nil, fmt.Errorf("no usable model profile: %w", err)
		}
		if view.ID == "" {
			return nil, fmt.Errorf("no model profile configured")
		}
	}
	return s.cached(view), nil
}

func (s *Switch) cached(v ModelProfileView) Provider {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c, ok := s.cache[v.ID]; ok && !v.UpdatedAt.After(c.updatedAt) {
		return c.prov
	}
	prov := s.build(v)
	s.cache[v.ID] = &cachedProvider{prov: prov, updatedAt: v.UpdatedAt}
	return prov
}

func (s *Switch) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error) {
	prov, err := s.providerFor(ctx)
	if err != nil {
		return Message{}, err
	}
	return prov.Chat(ctx, messages, tools)
}

// SupportsVision reflects the DEFAULT profile (used by unattended entry points
// and attachment gating before a run's profile is known).
func (s *Switch) SupportsVision() bool {
	view, err := s.src.DefaultModelProfile()
	if err != nil || view.ID == "" {
		return false
	}
	return s.cached(view).SupportsVision()
}
