package workflow

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func linearYAML() []byte {
	return []byte(`name: n
steps:
  - id: a
    tool: ta
    args:
      q: "{{input.text}}"
  - id: b
    tool: tb
    args:
      x: "{{a.result.ok}}"
`)
}

func TestRunExecutesLinearlyWithEvents(t *testing.T) {
	w, err := Parse(linearYAML())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var calls []string
	hooks := ExecHooks{
		Emit: func(typ string, d map[string]any) error {
			calls = append(calls, typ)
			return nil
		},
		Invoke: func(ctx context.Context, tool, stepID string, args map[string]any) (map[string]any, bool, error) {
			calls = append(calls, "invoke:"+tool)
			if tool == "ta" {
				return map[string]any{"ok": true}, false, nil
			}
			return map[string]any{}, false, nil
		},
	}
	tree := map[string]any{"input": map[string]any{"text": "hi"}}
	if err := w.Run(context.Background(), tree, hooks); err != nil {
		t.Fatalf("Run: %v", err)
	}
	want := []string{
		"workflow.started",
		"workflow.step_started", "invoke:ta", "workflow.step_completed",
		"workflow.step_started", "invoke:tb", "workflow.step_completed",
	}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls=%v want=%v", calls, want)
	}
	if v, ok := Resolve(tree, "a.result.ok"); !ok || v != true {
		t.Fatalf("tree[a.result.ok]=%v,%v want true", v, ok)
	}
}

func TestRunMissingPathFails(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    args:
      q: "{{input.text}}"
  - id: b
    tool: tb
    args:
      x: "{{a.result.nope}}"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var invokes int
	hooks := ExecHooks{
		Emit: func(typ string, d map[string]any) error { return nil },
		Invoke: func(ctx context.Context, tool, stepID string, args map[string]any) (map[string]any, bool, error) {
			invokes++
			return map[string]any{"ok": true}, false, nil
		},
	}
	err = w.Run(context.Background(), map[string]any{"input": map[string]any{"text": "hi"}}, hooks)
	if err == nil {
		t.Fatal("expected missing-reference error")
	}
	if !strings.Contains(err.Error(), `step "b"`) {
		t.Fatalf("error missing failing step id: %v", err)
	}
	if !strings.Contains(err.Error(), "a.result.nope") {
		t.Fatalf("error missing unresolved path: %v", err)
	}
	if invokes != 1 {
		t.Fatalf("invokes=%d want 1 (fail fast before step b)", invokes)
	}
}

func TestRunApproveRejectStops(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    args:
      q: "{{input.text}}"
  - id: b
    tool: tb
    approve: true
    args:
      x: "{{a.result}}"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var invokes int
	hooks := ExecHooks{
		Emit: func(typ string, d map[string]any) error { return nil },
		Gate: func(ctx context.Context, p store.HITLPayload) (bool, error) {
			return false, nil
		},
		Invoke: func(ctx context.Context, tool, stepID string, args map[string]any) (map[string]any, bool, error) {
			invokes++
			return map[string]any{}, false, nil
		},
	}
	err = w.Run(context.Background(), map[string]any{"input": map[string]any{"text": "hi"}}, hooks)
	if err == nil {
		t.Fatal("expected rejected error")
	}
	if !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("error missing rejected: %v", err)
	}
	if !strings.Contains(err.Error(), `step "b"`) {
		t.Fatalf("error missing rejected step id: %v", err)
	}
	if invokes != 1 {
		t.Fatalf("invokes=%d want 1 (step a runs; step b stops at gate)", invokes)
	}
}

func TestRunApproveApproveContinues(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    approve: true
    args:
      q: "{{input.text}}"
      limit: 5
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var got store.HITLPayload
	called := false
	hooks := ExecHooks{
		Emit: func(typ string, d map[string]any) error { return nil },
		Gate: func(ctx context.Context, p store.HITLPayload) (bool, error) {
			called = true
			got = p
			return true, nil
		},
		Invoke: func(ctx context.Context, tool, stepID string, args map[string]any) (map[string]any, bool, error) {
			return map[string]any{"done": true}, false, nil
		},
	}
	if err := w.Run(context.Background(), map[string]any{"input": map[string]any{"text": "hi"}}, hooks); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !called {
		t.Fatal("gate hook never invoked")
	}
	if !strings.Contains(got.Prompt, "a") {
		t.Fatalf("prompt missing step id: %q", got.Prompt)
	}
	if got.ToolName != "ta" {
		t.Fatalf("payload tool=%q want ta", got.ToolName)
	}
	if !reflect.DeepEqual(got.Arguments, map[string]any{"q": "hi", "limit": 5}) {
		t.Fatalf("payload arguments=%v want rendered args", got.Arguments)
	}
}

func TestRunGateErrorFails(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    approve: true
    args: {}
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var invokes int
	hooks := ExecHooks{
		Emit: func(typ string, d map[string]any) error { return nil },
		Gate: func(ctx context.Context, p store.HITLPayload) (bool, error) {
			return false, context.Canceled
		},
		Invoke: func(ctx context.Context, tool, stepID string, args map[string]any) (map[string]any, bool, error) {
			invokes++
			return nil, false, nil
		},
	}
	err = w.Run(context.Background(), map[string]any{}, hooks)
	if err == nil {
		t.Fatal("expected gate error")
	}
	if !strings.Contains(err.Error(), `step "a"`) {
		t.Fatalf("error missing failing step id: %v", err)
	}
	if invokes != 0 {
		t.Fatalf("invokes=%d want 0 (gate error must stop)", invokes)
	}
}
