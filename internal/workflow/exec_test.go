package workflow

import (
	"context"
	"errors"
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

var errPause = errors.New("pause for approval")

func TestRunFromPausesAtApproval(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
  - id: b
    tool: tb
    approve: true
  - id: c
    tool: tc
`))
	if err != nil {
		t.Fatal(err)
	}
	var invoked []string
	var paused State
	hooks := ExecHooks{
		Emit: func(string, map[string]any) error { return nil },
		GateState: func(_ context.Context, p store.HITLPayload, st State) (bool, error) {
			paused = st
			if p.ToolName != "tb" {
				t.Fatalf("tool=%q", p.ToolName)
			}
			return false, errPause
		},
		Invoke: func(_ context.Context, tool, stepID string, _ map[string]any) (map[string]any, bool, error) {
			invoked = append(invoked, stepID)
			return map[string]any{"ok": true}, false, nil
		},
	}
	tree := map[string]any{"input": map[string]any{"text": "hi"}}
	_, err = w.RunFrom(context.Background(), State{Skill: "n", NextStep: 0, Tree: tree}, hooks)
	if !errors.Is(err, errPause) {
		t.Fatalf("err=%v want pause", err)
	}
	if !reflect.DeepEqual(invoked, []string{"a"}) {
		t.Fatalf("invoked=%v want [a]", invoked)
	}
	if paused.Skill != "n" || paused.NextStep != 1 || paused.StepID != "b" {
		t.Fatalf("paused=%+v", paused)
	}
	if v, ok := Resolve(paused.Tree, "a.result.ok"); !ok || v != true {
		t.Fatalf("tree lost step a result")
	}
}

func TestRunFromResumesAtNextStep(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
  - id: b
    tool: tb
    approve: true
  - id: c
    tool: tc
    args:
      x: "{{b.result.id}}"
`))
	if err != nil {
		t.Fatal(err)
	}
	tree := map[string]any{
		"input": map[string]any{"text": "hi"},
		"a":     map[string]any{"result": map[string]any{"ok": true}},
		"b":     map[string]any{"result": map[string]any{"id": "9"}},
	}
	var events []string
	var invoked []string
	hooks := ExecHooks{
		Emit: func(typ string, _ map[string]any) error { events = append(events, typ); return nil },
		GateState: func(context.Context, store.HITLPayload, State) (bool, error) {
			t.Fatal("approved step must not gate again")
			return false, nil
		},
		Invoke: func(_ context.Context, tool, stepID string, args map[string]any) (map[string]any, bool, error) {
			invoked = append(invoked, stepID)
			if stepID == "c" && args["x"] != "9" {
				t.Fatalf("args=%v", args)
			}
			return map[string]any{"done": true}, false, nil
		},
	}
	_, err = w.RunFrom(context.Background(), State{Skill: "n", NextStep: 2, Tree: tree}, hooks)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(invoked, []string{"c"}) {
		t.Fatalf("invoked=%v want [c]", invoked)
	}
	if len(events) == 0 || events[0] != "workflow.resumed" {
		t.Fatalf("events=%v want workflow.resumed first", events)
	}
}

func TestRunRetriesFailedStep(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    retry: 2
`))
	if err != nil {
		t.Fatal(err)
	}
	var invokes int
	var retries int
	hooks := ExecHooks{
		Emit: func(typ string, _ map[string]any) error {
			if typ == "workflow.step_retry" {
				retries++
			}
			return nil
		},
		Invoke: func(context.Context, string, string, map[string]any) (map[string]any, bool, error) {
			invokes++
			if invokes < 3 {
				return map[string]any{"error": "transient"}, true, nil
			}
			return map[string]any{"ok": true}, false, nil
		},
	}
	if err := w.Run(context.Background(), map[string]any{}, hooks); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if invokes != 3 || retries != 2 {
		t.Fatalf("invokes=%d retries=%d want 3/2", invokes, retries)
	}
}

func TestRunRetryExhaustedFails(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    retry: 1
`))
	if err != nil {
		t.Fatal(err)
	}
	invokes := 0
	hooks := ExecHooks{
		Emit: func(string, map[string]any) error { return nil },
		Invoke: func(context.Context, string, string, map[string]any) (map[string]any, bool, error) {
			invokes++
			return map[string]any{}, true, nil
		},
	}
	err = w.Run(context.Background(), map[string]any{}, hooks)
	if err == nil || !strings.Contains(err.Error(), "is_error") {
		t.Fatalf("err=%v", err)
	}
	if invokes != 2 {
		t.Fatalf("invokes=%d want 2", invokes)
	}
}

func TestRunRetryStopsOnNoRetry(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    retry: 3
`))
	if err != nil {
		t.Fatal(err)
	}
	invokes := 0
	sentinel := errors.New("waiting human")
	hooks := ExecHooks{
		Emit: func(string, map[string]any) error { return nil },
		Invoke: func(context.Context, string, string, map[string]any) (map[string]any, bool, error) {
			invokes++
			return nil, false, errors.Join(ErrNoRetry, sentinel)
		},
	}
	err = w.Run(context.Background(), map[string]any{}, hooks)
	if invokes != 1 {
		t.Fatalf("invokes=%d want 1", invokes)
	}
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v lost sentinel", err)
	}
}

func TestRunStepTimeoutFails(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    timeout_ms: 20
`))
	if err != nil {
		t.Fatal(err)
	}
	timeouts := 0
	hooks := ExecHooks{
		Emit: func(typ string, _ map[string]any) error {
			if typ == "workflow.step_timeout" {
				timeouts++
			}
			return nil
		},
		Invoke: func(ctx context.Context, _ string, _ string, _ map[string]any) (map[string]any, bool, error) {
			<-ctx.Done()
			return nil, false, ctx.Err()
		},
	}
	err = w.Run(context.Background(), map[string]any{}, hooks)
	if err == nil || !strings.Contains(err.Error(), "timed out after 20ms") {
		t.Fatalf("err=%v", err)
	}
	if timeouts != 1 {
		t.Fatalf("timeouts=%d want 1", timeouts)
	}
}

func TestRunStepTimeoutRetries(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    retry: 1
    timeout_ms: 20
`))
	if err != nil {
		t.Fatal(err)
	}
	invokes := 0
	retries := 0
	hooks := ExecHooks{
		Emit: func(typ string, _ map[string]any) error {
			if typ == "workflow.step_retry" {
				retries++
			}
			return nil
		},
		Invoke: func(ctx context.Context, _ string, _ string, _ map[string]any) (map[string]any, bool, error) {
			invokes++
			<-ctx.Done()
			return nil, false, ctx.Err()
		},
	}
	err = w.Run(context.Background(), map[string]any{}, hooks)
	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("err=%v", err)
	}
	if invokes != 2 || retries != 1 {
		t.Fatalf("invokes=%d retries=%d want 2/1", invokes, retries)
	}
}

func TestRunStepTimeoutFastSuccess(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    timeout_ms: 5000
`))
	if err != nil {
		t.Fatal(err)
	}
	hooks := ExecHooks{
		Emit: func(string, map[string]any) error { return nil },
		Invoke: func(context.Context, string, string, map[string]any) (map[string]any, bool, error) {
			return map[string]any{"ok": true}, false, nil
		},
	}
	if err := w.Run(context.Background(), map[string]any{}, hooks); err != nil {
		t.Fatalf("Run: %v", err)
	}
}

func TestRunContinueOnErrorKeepsGoing(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    continue_on_error: true
  - id: b
    tool: tb
`))
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	var invoked []string
	hooks := ExecHooks{
		Emit: func(typ string, _ map[string]any) error { events = append(events, typ); return nil },
		Invoke: func(_ context.Context, _ string, stepID string, _ map[string]any) (map[string]any, bool, error) {
			invoked = append(invoked, stepID)
			if stepID == "a" {
				return nil, true, nil
			}
			return map[string]any{"ok": true}, false, nil
		},
	}
	tree := map[string]any{}
	if err := w.Run(context.Background(), tree, hooks); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !reflect.DeepEqual(invoked, []string{"a", "b"}) {
		t.Fatalf("invoked=%v want [a b]", invoked)
	}
	got, ok := Resolve(tree, "a.result.is_error")
	if !ok || got != true {
		t.Fatalf("a.result.is_error=%v,%v want true", got, ok)
	}
	if !reflect.DeepEqual(events, []string{
		"workflow.started",
		"workflow.step_started", "workflow.step_failed", "workflow.step_completed",
		"workflow.step_started", "workflow.step_completed",
	}) {
		t.Fatalf("events=%v", events)
	}
}

func TestRunContinueOnErrorStopsOnPause(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
    continue_on_error: true
`))
	if err != nil {
		t.Fatal(err)
	}
	invokes := 0
	hooks := ExecHooks{
		Emit: func(string, map[string]any) error { return nil },
		Invoke: func(context.Context, string, string, map[string]any) (map[string]any, bool, error) {
			invokes++
			return nil, false, errors.Join(ErrNoRetry, errors.New("waiting human"))
		},
	}
	if err := w.Run(context.Background(), map[string]any{}, hooks); err == nil {
		t.Fatal("want pause error")
	}
	if invokes != 1 {
		t.Fatalf("invokes=%d want 1", invokes)
	}
}
