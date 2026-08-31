package llm

import (
	"context"
	"testing"
	"time"
)

// fakeProfileSource is an in-memory ProfileSource for tests.
type fakeProfileSource struct {
	def  ModelProfileView
	byID map[string]ModelProfileView
}

func (f *fakeProfileSource) DefaultModelProfile() (ModelProfileView, error) { return f.def, nil }
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
	src.def = ModelProfileView{ID: "mp_def", Model: "model-def"}

	ctx := WithModelProfileID(context.Background(), "mp_a")
	msg, err := sw.Chat(ctx, nil, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if msg.Content != "from:model-A" {
		t.Fatalf("did not use explicit profile: %q", msg.Content)
	}
	// Default profile in this test has zero-value SupportsVision=false.
	if sw.SupportsVision() {
		t.Fatal("SupportsVision should reflect default profile (false here)")
	}
}

func TestSwitchFallsBackToDefault(t *testing.T) {
	src := &fakeProfileSource{
		def:  ModelProfileView{ID: "mp_def", Model: "model-def", SupportsVision: true},
		byID: map[string]ModelProfileView{},
	}
	sw := NewSwitch(src)
	used := ""
	sw.build = func(v ModelProfileView) Provider {
		used = v.Model
		return &recordingProvider{model: v.Model, vision: v.SupportsVision}
	}

	// No profile id in ctx -> default.
	if _, err := sw.Chat(context.Background(), nil, nil); err != nil {
		t.Fatalf("chat: %v", err)
	}
	if used != "model-def" {
		t.Fatalf("want default model-def, used %q", used)
	}
	// Deleted profile id -> default.
	ctx := WithModelProfileID(context.Background(), "mp_gone")
	if _, err := sw.Chat(ctx, nil, nil); err != nil {
		t.Fatalf("chat with missing profile should fall back, got %v", err)
	}
	if used != "model-def" {
		t.Fatalf("missing profile should fall back to default, used %q", used)
	}
	if !sw.SupportsVision() {
		t.Fatal("SupportsVision must reflect default profile vision=true")
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
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "v1", UpdatedAt: t0}
	src.def = src.byID["mp_a"]

	ctx := WithModelProfileID(context.Background(), "mp_a")
	sw.Chat(ctx, nil, nil)
	sw.Chat(ctx, nil, nil) // cached, no rebuild
	// profile edited: UpdatedAt advances.
	src.byID["mp_a"] = ModelProfileView{ID: "mp_a", Model: "v2", UpdatedAt: t0.Add(time.Second)}
	src.def = src.byID["mp_a"]
	sw.Chat(ctx, nil, nil)

	if len(models) != 2 || models[0] != "v1" || models[1] != "v2" {
		t.Fatalf("expected rebuild after update (v1 then v2), got %v", models)
	}
}
