package llm

import (
	"context"
	"testing"
	"time"
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
	sw.Chat(ctx, nil, nil)
	sw.Chat(ctx, nil, nil) // cached, no rebuild
	// profile edited: UpdatedAt advances.
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "v2", Tier: "standard", UpdatedAt: t0.Add(time.Second)}
	src.list = []ModelProfileView{src.byID["mp_a"]}
	sw.Chat(ctx, nil, nil)

	if len(models) != 2 || models[0] != "v1" || models[1] != "v2" {
		t.Fatalf("expected rebuild after update (v1 then v2), got %v", models)
	}
}
