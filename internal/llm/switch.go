package llm

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
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
	ThinkingLevel   string
	ThinkingDialect string
	SupportsVision  bool
	SupportsPDF     bool
	ContextTokens   int
	Tier            string
	// FallbackProfileIDs is an ordered list of profile ids to try when the
	// primary call fails after its own retries are exhausted.
	FallbackProfileIDs []string
	UpdatedAt          time.Time
}

// ProfileSource resolves model profiles (backed by store.Store in production).
type ProfileSource interface {
	ListProfiles() ([]ModelProfileView, error)
	ModelProfileByID(id string) (ModelProfileView, error)
}

type ctxKey int

const (
	modelProfileIDKey ctxKey = iota
	thinkingLevelKey
)

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

// WithThinkingLevel attaches a per-run thinking level override to the context.
func WithThinkingLevel(ctx context.Context, level string) context.Context {
	if level == "" {
		return ctx
	}
	return context.WithValue(ctx, thinkingLevelKey, level)
}

// ThinkingLevelFromContext returns the per-run thinking level, or "".
func ThinkingLevelFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(thinkingLevelKey).(string); ok {
		return v
	}
	return ""
}

type cachedProvider struct {
	prov      Provider
	updatedAt time.Time
}

// errChoiceUnsupported is returned when a tool_choice constraint was requested
// (DP-2b) but the resolved provider cannot enforce it. Callers treat it as the
// signal to fail open to an ordinary call.
var errChoiceUnsupported = errors.New("llm: provider does not support tool_choice constraint")

// Switch is a Provider that resolves the active model per-run from a
// ProfileSource. Provider instances are cached by profile id and rebuilt when
// the profile's UpdatedAt advances (hot reload, no restart).
type Switch struct {
	src   ProfileSource
	mu    sync.Mutex
	cache map[string]*cachedProvider

	// openAIMaxRetries, when non-nil, overrides NewOpenAI's default MaxRetries
	// for providers built by defaultBuild (0 disables retries).
	openAIMaxRetries *int

	// Optional timeout overrides applied in defaultBuild.
	openAIConnectTimeout        time.Duration
	openAIResponseHeaderTimeout time.Duration
	openAIStreamIdleTimeout     time.Duration
	openAIMaxRequestTimeout     time.Duration

	// build constructs a Provider from a profile. Overridable in tests.
	build func(ModelProfileView) Provider
	// fallbackBlocked optionally returns profiles that must not be used as
	// fallbacks (for example, models whose budget is exhausted).
	fallbackBlocked func() map[string]bool
}

func NewSwitch(src ProfileSource) *Switch {
	s := &Switch{src: src, cache: map[string]*cachedProvider{}}
	s.build = s.defaultBuild
	return s
}

// SetFallbackBlocked installs an optional filter for fallback profiles. The
// callback is evaluated once per fallback plan; nil clears the filter.
func (s *Switch) SetFallbackBlocked(fn func() map[string]bool) {
	s.fallbackBlocked = fn
}

// SetOpenAIMaxRetries applies llm.api_max_retries to OpenAI clients built by
// this Switch. Pass 0 to disable retries (pre-P0 behaviour).
func (s *Switch) SetOpenAIMaxRetries(n int) {
	s.openAIMaxRetries = &n
}

// SetOpenAITimeouts applies llm connect / header / stream-idle / max-request
// timeouts (seconds already converted to durations by the caller). Zero values
// leave NewOpenAI defaults unchanged.
func (s *Switch) SetOpenAITimeouts(connect, responseHeader, streamIdle, maxRequest time.Duration) {
	s.openAIConnectTimeout = connect
	s.openAIResponseHeaderTimeout = responseHeader
	s.openAIStreamIdleTimeout = streamIdle
	s.openAIMaxRequestTimeout = maxRequest
}

func (s *Switch) defaultBuild(v ModelProfileView) Provider {
	key := v.APIKey
	if key == "" && v.APIKeyEnv != "" {
		key = os.Getenv(v.APIKeyEnv)
	}
	p := NewOpenAI(v.BaseURL, key, v.Model)
	p.DisableThinking = v.DisableThinking
	p.ThinkingLevel = v.ThinkingLevel
	p.ThinkingDialect = v.ThinkingDialect
	p.VisionSupported = v.SupportsVision
	if s.openAIMaxRetries != nil {
		p.MaxRetries = *s.openAIMaxRetries
	}
	rebuild := false
	if s.openAIConnectTimeout > 0 {
		p.ConnectTimeout = s.openAIConnectTimeout
		rebuild = true
	}
	if s.openAIResponseHeaderTimeout > 0 {
		p.ResponseHeaderTimeout = s.openAIResponseHeaderTimeout
		rebuild = true
	}
	if s.openAIStreamIdleTimeout > 0 {
		p.StreamIdleTimeout = s.openAIStreamIdleTimeout
	}
	if s.openAIMaxRequestTimeout > 0 {
		p.MaxRequestTimeout = s.openAIMaxRequestTimeout
	}
	if rebuild {
		p.RebuildHTTPClient()
	}
	return p
}

func (s *Switch) viewFor(ctx context.Context) (ModelProfileView, error) {
	id := ModelProfileIDFromContext(ctx)
	if id != "" {
		if view, err := s.src.ModelProfileByID(id); err == nil && view.ID != "" {
			return view, nil
		}
	}
	// No/invalid per-run id: use the Auto primary model (earliest standard-tier
	// profile, else the earliest profile). This is both the Auto fallback for
	// text and the target for background callers without a pinned run id.
	list, err := s.src.ListProfiles()
	if err != nil {
		return ModelProfileView{}, fmt.Errorf("no usable model profile: %w", err)
	}
	view, err := PrimaryModelProfile(list)
	if err != nil {
		return ModelProfileView{}, fmt.Errorf("no usable model profile: %w", err)
	}
	return view, nil
}

func (s *Switch) providerFor(ctx context.Context) (Provider, error) {
	view, err := s.viewFor(ctx)
	if err != nil {
		return nil, err
	}
	return s.cached(view), nil
}

// messageNeeds reports which model capabilities the outgoing turn requires.
type messageNeeds struct {
	vision bool
	pdf    bool
}

func messageNeedsOf(messages []Message) messageNeeds {
	var n messageNeeds
	for _, m := range messages {
		for _, part := range m.Parts {
			switch part.Type {
			case "image":
				n.vision = true
			case "file":
				// Only native PDF parts exist today; add more capability flags
				// here if other document types are introduced.
				n.pdf = true
			}
		}
	}
	return n
}

func (n messageNeeds) label() string {
	switch {
	case n.pdf && n.vision:
		return "vision and PDF"
	case n.pdf:
		return "PDF"
	case n.vision:
		return "vision"
	}
	return ""
}

// fallbackPlan is the primary profile plus the fallback profiles that can
// handle this turn's capabilities.
type fallbackPlan struct {
	chain         []ModelProfileView
	needs         messageNeeds
	skipped       int
	budgetSkipped int
}

// wrap adds a capability note when fallbacks were filtered out and the primary
// still failed, so the error explains why no other model was tried.
func (p fallbackPlan) wrap(err error) error {
	if err == nil {
		return err
	}
	if p.skipped > 0 {
		err = fmt.Errorf("%w; %d fallback model(s) skipped because they do not support %s",
			err, p.skipped, p.needs.label())
	}
	if p.budgetSkipped > 0 {
		err = fmt.Errorf("%w; %d fallback model(s) skipped because their budget is exhausted",
			err, p.budgetSkipped)
	}
	return err
}

// fallbackPlan returns the primary profile followed by its FallbackProfileIDs
// (skipping missing/cyclic ids). Fallbacks that cannot accept the turn's image
// or PDF parts are skipped so a document is never handed to a model that would
// ignore or reject it.
func (s *Switch) resolveFallbackPlan(ctx context.Context, messages []Message) (fallbackPlan, error) {
	needs := messageNeedsOf(messages)
	primary, err := s.viewFor(ctx)
	if err != nil {
		return fallbackPlan{needs: needs}, err
	}
	plan := fallbackPlan{chain: []ModelProfileView{primary}, needs: needs}
	var blocked map[string]bool
	if s.fallbackBlocked != nil {
		blocked = s.fallbackBlocked()
	}
	seen := map[string]bool{primary.ID: true}
	for _, id := range primary.FallbackProfileIDs {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		view, err := s.src.ModelProfileByID(id)
		if err != nil || view.ID == "" {
			continue
		}
		seen[id] = true
		if blocked[id] {
			plan.budgetSkipped++
			continue
		}
		if (needs.vision && !view.SupportsVision) || (needs.pdf && !view.SupportsPDF) {
			plan.skipped++
			continue
		}
		plan.chain = append(plan.chain, view)
	}
	return plan, nil
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
	plan, err := s.resolveFallbackPlan(ctx, messages)
	if err != nil {
		return Message{}, err
	}
	var lastErr error
	for _, view := range plan.chain {
		msg, err := s.cached(view).Chat(ctx, messages, tools)
		if err == nil {
			return msg, nil
		}
		lastErr = err
	}
	return Message{}, plan.wrap(lastErr)
}

// ChatWithChoice implements Chooser (DP-2b) by forwarding to the resolved
// provider when it supports the optional capability. If the backing provider
// cannot enforce a tool_choice, we return an error rather than silently
// downgrading to Chat 鈥?dropping the constraint would make the call look
// constrained when it was not.
func (s *Switch) ChatWithChoice(ctx context.Context, messages []Message, tools []ToolSpec, choice ToolChoice) (Message, error) {
	plan, err := s.resolveFallbackPlan(ctx, messages)
	if err != nil {
		return Message{}, err
	}
	var lastErr error
	var sawUnsupported bool
	for _, view := range plan.chain {
		prov := s.cached(view)
		ch, ok := prov.(Chooser)
		if !ok {
			sawUnsupported = true
			continue
		}
		msg, err := ch.ChatWithChoice(ctx, messages, tools, choice)
		if err == nil {
			return msg, nil
		}
		lastErr = err
	}
	if lastErr != nil {
		return Message{}, plan.wrap(lastErr)
	}
	if sawUnsupported {
		return Message{}, errChoiceUnsupported
	}
	return Message{}, fmt.Errorf("no usable model profile")
}

// ChatStream implements Streamer: prefers a streaming provider, otherwise Chat.
func (s *Switch) ChatStream(ctx context.Context, messages []Message, tools []ToolSpec, onThink, onContent func(cumulative string)) (Message, error) {
	plan, err := s.resolveFallbackPlan(ctx, messages)
	if err != nil {
		return Message{}, err
	}
	var lastErr error
	for _, view := range plan.chain {
		p := s.cached(view)
		var msg Message
		var err error
		if st, ok := p.(Streamer); ok {
			msg, err = st.ChatStream(ctx, messages, tools, onThink, onContent)
		} else {
			msg, err = p.Chat(ctx, messages, tools)
		}
		if err == nil {
			return msg, nil
		}
		lastErr = err
	}
	return Message{}, plan.wrap(lastErr)
}

// ChatStreamWithChoice implements StreamChooser (DP-2b). Prefers a streaming
// constrained call; otherwise falls back to a constrained non-streaming call
// (still enforcing the choice), never to an unconstrained one.
func (s *Switch) ChatStreamWithChoice(
	ctx context.Context,
	messages []Message,
	tools []ToolSpec,
	choice ToolChoice,
	onThink, onContent func(cumulative string),
) (Message, error) {
	plan, err := s.resolveFallbackPlan(ctx, messages)
	if err != nil {
		return Message{}, err
	}
	var lastErr error
	var sawUnsupported bool
	for _, view := range plan.chain {
		p := s.cached(view)
		if sc, ok := p.(StreamChooser); ok {
			msg, err := sc.ChatStreamWithChoice(ctx, messages, tools, choice, onThink, onContent)
			if err == nil {
				return msg, nil
			}
			lastErr = err
			continue
		}
		if c, ok := p.(Chooser); ok {
			msg, err := c.ChatWithChoice(ctx, messages, tools, choice)
			if err == nil {
				return msg, nil
			}
			lastErr = err
			continue
		}
		sawUnsupported = true
	}
	if lastErr != nil {
		return Message{}, plan.wrap(lastErr)
	}
	if sawUnsupported {
		return Message{}, errChoiceUnsupported
	}
	return Message{}, fmt.Errorf("no usable model profile")
}

// SupportsVision reports whether ANY configured model can accept image parts.
// Under task-aware Auto an image turn is routed to a vision-capable model when
// one exists, so this is the meaningful capability signal for attachment
// gating (rather than one fixed "default" model's capability).
func (s *Switch) SupportsVision() bool {
	list, err := s.src.ListProfiles()
	if err != nil {
		return false
	}
	for _, v := range list {
		if v.SupportsVision {
			return true
		}
	}
	return false
}
