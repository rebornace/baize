package llm

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

// fakeProfileSource is an in-memory ProfileSource for tests.
type fakeProfileSource struct {
	list []ModelProfileView
	byID map[string]ModelProfileView
}

func (f *fakeProfileSource) ListProfiles() ([]ModelProfileView, error) { return f.list, nil }
func (f *fakeProfileSource) ModelProfileByID(id string) (ModelProfileView, error) {
	p, ok := f.byID[id]
	if !ok {
		return ModelProfileView{}, context.Canceled // any non-nil error signals missing
	}
	return p, nil
}

// recordingProvider captures the model name it was built with.
type recordingProvider struct {
	model  string
	vision bool
}

func (r *recordingProvider) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error) {
	return Message{Content: "from:" + r.model}, nil
}
func (r *recordingProvider) SupportsVision() bool { return r.vision }

func TestSwitchResolvesExplicitProfile(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	sw := NewSwitch(src)
	sw.build = func(v ModelProfileView) Provider {
		return &recordingProvider{model: v.Model, vision: v.SupportsVision}
	}
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "model-A", SupportsVision: true, UpdatedAt: time.Now()}
	src.list = []ModelProfileView{{ID: "mp_def", Model: "model-def"}}

	ctx := WithModelProfileID(context.Background(), "mp_a")
	msg, err := sw.Chat(ctx, nil, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if msg.Content != "from:model-A" {
		t.Fatalf("did not use explicit profile: %q", msg.Content)
	}
	// No vision model in the list -> SupportsVision false.
	if sw.SupportsVision() {
		t.Fatal("SupportsVision should be false when no vision model exists")
	}
}

func TestSwitchFallsBackToPrimary(t *testing.T) {
	src := &fakeProfileSource{
		list: []ModelProfileView{
			{ID: "mp_def", Model: "model-def", Tier: "standard", SupportsVision: true},
		},
		byID: map[string]ModelProfileView{},
	}
	sw := NewSwitch(src)
	used := ""
	sw.build = func(v ModelProfileView) Provider {
		used = v.Model
		return &recordingProvider{model: v.Model, vision: v.SupportsVision}
	}

	// No profile id in ctx -> primary (earliest standard-tier).
	if _, err := sw.Chat(context.Background(), nil, nil); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if used != "model-def" {
		t.Fatalf("want primary model-def, used %q", used)
	}
	// Deleted profile id -> primary.
	ctx := WithModelProfileID(context.Background(), "mp_gone")
	if _, err := sw.Chat(ctx, nil, nil); err != nil {
		t.Fatalf("chat with missing profile should fall back, got %v", err)
	}
	if used != "model-def" {
		t.Fatalf("missing profile should fall back to primary, used %q", used)
	}
	if !sw.SupportsVision() {
		t.Fatal("SupportsVision must be true when a vision model exists")
	}
}

func TestSwitchNoProfileErrors(t *testing.T) {
	sw := NewSwitch(&fakeProfileSource{})
	if _, err := sw.Chat(context.Background(), nil, nil); err == nil {
		t.Fatal("chat with no configured model must error")
	}
	if sw.SupportsVision() {
		t.Fatal("SupportsVision false with no models")
	}
}

func TestSwitchRebuildsOnUpdate(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	sw := NewSwitch(src)
	models := []string{}
	sw.build = func(v ModelProfileView) Provider {
		models = append(models, v.Model)
		return &recordingProvider{model: v.Model}
	}
	t0 := time.Now()
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "v1", Tier: "standard", UpdatedAt: t0}
	src.list = []ModelProfileView{src.byID["mp_a"]}

	ctx := WithModelProfileID(context.Background(), "mp_a")
	if _, err := sw.Chat(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := sw.Chat(ctx, nil, nil); err != nil { // cached, no rebuild
		t.Fatal(err)
	}
	// profile edited: UpdatedAt advances.
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "v2", Tier: "standard", UpdatedAt: t0.Add(time.Second)}
	src.list = []ModelProfileView{src.byID["mp_a"]}
	if _, err := sw.Chat(ctx, nil, nil); err != nil {
		t.Fatal(err)
	}

	if len(models) != 2 || models[0] != "v1" || models[1] != "v2" {
		t.Fatalf("expected rebuild after update (v1 then v2), got %v", models)
	}
}

func TestSwitchFallbackSecondProfile(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	src.byID["mp_a"] = ModelProfileView{
		ID:                 "mp_a",
		Model:              "primary",
		FallbackProfileIDs: []string{"mp_b"},
	}
	src.byID["mp_b"] = ModelProfileView{ID: "mp_b", Model: "fallback"}
	src.list = []ModelProfileView{src.byID["mp_a"], src.byID["mp_b"]}

	sw := NewSwitch(src)
	sw.build = func(v ModelProfileView) Provider {
		if v.ID == "mp_a" {
			return &failingProvider{err: context.DeadlineExceeded}
		}
		return &recordingProvider{model: v.Model}
	}

	ctx := WithModelProfileID(context.Background(), "mp_a")
	msg, err := sw.Chat(ctx, nil, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if msg.Content != "from:fallback" {
		t.Fatalf("want fallback, got %q", msg.Content)
	}
}

type failingProvider struct{ err error }

func (f *failingProvider) Chat(ctx context.Context, messages []Message, tools []ToolSpec) (Message, error) {
	return Message{}, f.err
}
func (f *failingProvider) SupportsVision() bool { return false }

func TestSwitchFallbackSkipsNonPDFModelForPDFTurn(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	src.byID["mp_a"] = ModelProfileView{
		ID:                 "mp_a",
		Model:              "primary",
		SupportsPDF:        true,
		FallbackProfileIDs: []string{"mp_text", "mp_pdf"},
	}
	src.byID["mp_text"] = ModelProfileView{ID: "mp_text", Model: "text-only"}
	src.byID["mp_pdf"] = ModelProfileView{ID: "mp_pdf", Model: "pdf-capable", SupportsPDF: true}
	src.list = []ModelProfileView{src.byID["mp_a"], src.byID["mp_text"], src.byID["mp_pdf"]}

	sw := NewSwitch(src)
	used := []string{}
	sw.build = func(v ModelProfileView) Provider {
		used = append(used, v.Model)
		if v.ID == "mp_a" {
			return &failingProvider{err: context.DeadlineExceeded}
		}
		return &recordingProvider{model: v.Model}
	}

	msgs := []Message{{Role: RoleUser, Parts: []ContentPart{{Type: "file", FileMIME: "application/pdf"}}}}
	ctx := WithModelProfileID(context.Background(), "mp_a")
	msg, err := sw.Chat(ctx, msgs, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if msg.Content != "from:pdf-capable" {
		t.Fatalf("want pdf fallback, got %q", msg.Content)
	}
	if len(used) != 2 || used[0] != "primary" || used[1] != "pdf-capable" {
		t.Fatalf("text-only fallback should have been skipped, used=%v", used)
	}
}

func TestSwitchFallbackSkipsNonVisionModelForImageTurn(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	src.byID["mp_a"] = ModelProfileView{
		ID:                 "mp_a",
		Model:              "primary",
		SupportsVision:     true,
		FallbackProfileIDs: []string{"mp_text", "mp_vis"},
	}
	src.byID["mp_text"] = ModelProfileView{ID: "mp_text", Model: "text-only"}
	src.byID["mp_vis"] = ModelProfileView{ID: "mp_vis", Model: "vision", SupportsVision: true}
	src.list = []ModelProfileView{src.byID["mp_a"], src.byID["mp_text"], src.byID["mp_vis"]}

	sw := NewSwitch(src)
	used := []string{}
	sw.build = func(v ModelProfileView) Provider {
		used = append(used, v.Model)
		if v.ID == "mp_a" {
			return &failingProvider{err: context.DeadlineExceeded}
		}
		return &recordingProvider{model: v.Model}
	}

	msgs := []Message{{Role: RoleUser, Parts: []ContentPart{{Type: "image", ImageMIME: "image/png"}}}}
	ctx := WithModelProfileID(context.Background(), "mp_a")
	msg, err := sw.Chat(ctx, msgs, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if msg.Content != "from:vision" {
		t.Fatalf("want vision fallback, got %q", msg.Content)
	}
	if len(used) != 2 || used[0] != "primary" || used[1] != "vision" {
		t.Fatalf("text-only fallback should have been skipped, used=%v", used)
	}
}

func TestSwitchFallbackExplainsSkippedCapability(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	src.byID["mp_a"] = ModelProfileView{
		ID:                 "mp_a",
		Model:              "primary",
		SupportsPDF:        true,
		FallbackProfileIDs: []string{"mp_text"},
	}
	src.byID["mp_text"] = ModelProfileView{ID: "mp_text", Model: "text-only"}
	src.list = []ModelProfileView{src.byID["mp_a"], src.byID["mp_text"]}

	sw := NewSwitch(src)
	sw.build = func(v ModelProfileView) Provider {
		return &failingProvider{err: context.DeadlineExceeded}
	}

	msgs := []Message{{Role: RoleUser, Parts: []ContentPart{{Type: "file", FileMIME: "application/pdf"}}}}
	ctx := WithModelProfileID(context.Background(), "mp_a")
	_, err := sw.Chat(ctx, msgs, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error should wrap the primary failure: %v", err)
	}
	if !strings.Contains(err.Error(), "do not support PDF") {
		t.Fatalf("error should explain the skipped fallback: %v", err)
	}
}

func TestSwitchTextTurnKeepsTextOnlyFallback(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	src.byID["mp_a"] = ModelProfileView{
		ID:                 "mp_a",
		Model:              "primary",
		SupportsPDF:        true,
		FallbackProfileIDs: []string{"mp_text"},
	}
	src.byID["mp_text"] = ModelProfileView{ID: "mp_text", Model: "text-only"}
	src.list = []ModelProfileView{src.byID["mp_a"], src.byID["mp_text"]}

	sw := NewSwitch(src)
	sw.build = func(v ModelProfileView) Provider {
		if v.ID == "mp_a" {
			return &failingProvider{err: context.DeadlineExceeded}
		}
		return &recordingProvider{model: v.Model}
	}

	ctx := WithModelProfileID(context.Background(), "mp_a")
	msg, err := sw.Chat(ctx, []Message{{Role: RoleUser, Content: "hello"}}, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if msg.Content != "from:text-only" {
		t.Fatalf("text turn should use text-only fallback, got %q", msg.Content)
	}
}

func TestToViewCopiesPDFCapability(t *testing.T) {
	v := toView(store.ModelProfile{SupportsPDF: true})
	if !v.SupportsPDF {
		t.Fatal("toView must copy SupportsPDF so fallback filtering sees it")
	}
}

func TestSwitchDefaultBuildCopiesThinkingFields(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	sw := NewSwitch(src)
	view := ModelProfileView{
		ID:              "mp_t",
		BaseURL:         "https://x/v1",
		Model:           "m",
		APIKey:          "k",
		ThinkingLevel:   ThinkingHigh,
		ThinkingDialect: DialectDeepSeek,
		DisableThinking: false,
		UpdatedAt:       time.Now(),
	}
	src.byID["mp_t"] = view
	src.list = []ModelProfileView{view}

	ctx := WithModelProfileID(context.Background(), "mp_t")
	prov, err := sw.providerFor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	o, ok := prov.(*OpenAI)
	if !ok {
		t.Fatalf("want *OpenAI, got %T", prov)
	}
	if o.ThinkingLevel != ThinkingHigh || o.ThinkingDialect != DialectDeepSeek {
		t.Fatalf("thinking fields not copied: level=%q dialect=%q", o.ThinkingLevel, o.ThinkingDialect)
	}
}

func TestFallbackSkipsBudgetBlockedProfiles(t *testing.T) {
	src := &fakeProfileSource{byID: map[string]ModelProfileView{}}
	src.byID["mp_a"] = ModelProfileView{
		ID:                 "mp_a",
		Model:              "primary",
		FallbackProfileIDs: []string{"mp_b", "mp_c"},
	}
	src.byID["mp_b"] = ModelProfileView{ID: "mp_b", Model: "fallback-b"}
	src.byID["mp_c"] = ModelProfileView{ID: "mp_c", Model: "fallback-c"}
	src.list = []ModelProfileView{src.byID["mp_a"], src.byID["mp_b"], src.byID["mp_c"]}

	sw := NewSwitch(src)
	sw.SetFallbackBlocked(func() map[string]bool { return map[string]bool{"mp_b": true} })
	plan, err := sw.resolveFallbackPlan(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.chain) != 2 || plan.chain[1].ID != "mp_c" {
		t.Fatalf("chain=%+v", plan.chain)
	}
	if plan.budgetSkipped != 1 {
		t.Fatalf("budgetSkipped=%d", plan.budgetSkipped)
	}
	if msg := plan.wrap(errors.New("boom")).Error(); !strings.Contains(msg, "budget is exhausted") {
		t.Fatalf("wrap=%q", msg)
	}
}
