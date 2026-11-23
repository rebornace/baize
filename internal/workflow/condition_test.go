package workflow

import (
	"context"
	"reflect"
	"testing"
)

func conditionTree() map[string]any {
	return map[string]any{
		"input": map[string]any{"text": "hello world"},
		"a": map[string]any{"result": map[string]any{
			"ok":     true,
			"status": "open",
			"count":  3,
			"tags":   []any{"vpn", "urgent"},
		}},
	}
}

func TestConditionOperators(t *testing.T) {
	tree := conditionTree()
	cases := []struct {
		name string
		c    Condition
		want bool
	}{
		{"eq string", Condition{Path: "a.result.status", Op: "eq", Value: "open"}, true},
		{"ne string", Condition{Path: "a.result.status", Op: "ne", Value: "closed"}, true},
		{"eq number from string", Condition{Path: "a.result.count", Op: "eq", Value: "3"}, true},
		{"gt", Condition{Path: "a.result.count", Op: "gt", Value: 2}, true},
		{"lt false", Condition{Path: "a.result.count", Op: "lt", Value: 2}, false},
		{"contains string", Condition{Path: "input.text", Op: "contains", Value: "world"}, true},
		{"contains list", Condition{Path: "a.result.tags", Op: "contains", Value: "vpn"}, true},
		{"in", Condition{Path: "a.result.status", Op: "in", Value: []any{"open", "pending"}}, true},
		{"exists", Condition{Path: "a.result.ok", Op: "exists"}, true},
		{"not_exists", Condition{Path: "a.result.nope", Op: "not_exists"}, true},
		{"missing eq", Condition{Path: "a.result.nope", Op: "eq", Value: "x"}, false},
		{"all", Condition{All: []Condition{
			{Path: "a.result.ok", Op: "eq", Value: true},
			{Path: "a.result.status", Op: "eq", Value: "open"},
		}}, true},
		{"any", Condition{Any: []Condition{
			{Path: "a.result.status", Op: "eq", Value: "closed"},
			{Path: "a.result.count", Op: "gte", Value: 3},
		}}, true},
		{"not", Condition{Not: &Condition{Path: "a.result.ok", Op: "eq", Value: false}}, true},
	}
	for _, tc := range cases {
		got, err := EvalCondition(&tc.c, tree)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestConditionValidate(t *testing.T) {
	bad := []Condition{
		{},
		{Path: "a", Op: "nope"},
		{Path: "", Op: "eq", Value: 1},
		{Path: "a", Op: "eq"},
		{Path: "a", Op: "in", Value: "not-a-list"},
		{All: []Condition{{Path: "a", Op: "eq", Value: 1}, {Path: "b", Op: "eq", Value: 2}}, Any: []Condition{{Path: "c", Op: "eq", Value: 3}}},
	}
	for _, c := range bad {
		if err := c.Validate(); err == nil {
			t.Errorf("condition %+v should fail validation", c)
		}
	}
	good := Condition{All: []Condition{{Path: "a.result.ok", Op: "exists"}}}
	if err := good.Validate(); err != nil {
		t.Fatalf("good condition: %v", err)
	}
}

func TestWorkflowConditionSkipsStep(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
  - id: skip
    tool: tb
    when:
      path: a.result.ok
      op: eq
      value: false
  - id: run
    tool: tc
    when:
      all:
        - path: a.result.ok
          op: eq
          value: true
        - path: a.result.status
          op: eq
          value: open
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var events []string
	var invoked []string
	hooks := ExecHooks{
		Emit: func(typ string, d map[string]any) error {
			events = append(events, typ)
			return nil
		},
		Invoke: func(ctx context.Context, tool, stepID string, args map[string]any) (map[string]any, bool, error) {
			invoked = append(invoked, tool)
			return map[string]any{"ok": true, "status": "open"}, false, nil
		},
	}
	if err := w.Run(context.Background(), map[string]any{"input": map[string]any{"text": "hi"}}, hooks); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !reflect.DeepEqual(invoked, []string{"ta", "tc"}) {
		t.Fatalf("invoked=%v want ta,tc", invoked)
	}
	found := false
	for _, e := range events {
		if e == "workflow.step_skipped" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing workflow.step_skipped: %v", events)
	}
}

func TestWorkflowConditionMissingPathSkips(t *testing.T) {
	w, err := Parse([]byte(`name: n
steps:
  - id: a
    tool: ta
  - id: b
    tool: tb
    when:
      path: a.result.nope
      op: eq
      value: x
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var invoked int
	hooks := ExecHooks{
		Emit: func(string, map[string]any) error { return nil },
		Invoke: func(context.Context, string, string, map[string]any) (map[string]any, bool, error) {
			invoked++
			return map[string]any{}, false, nil
		},
	}
	if err := w.Run(context.Background(), map[string]any{"input": map[string]any{}}, hooks); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if invoked != 1 {
		t.Fatalf("invoked=%d want 1 (a only)", invoked)
	}
}
