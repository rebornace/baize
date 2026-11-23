package workflow_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/workflow"
)

func TestValidateReferences(t *testing.T) {
	ok := &workflow.Workflow{Name: "w", Steps: []workflow.Step{
		{ID: "lookup", Tool: "list", Args: map[string]any{"q": "{{input.query}}"}},
		{ID: "update", Tool: "update", Args: map[string]any{"id": "{{lookup.result.id}}"}},
	}}
	if err := workflow.ValidateReferences(ok); err != nil {
		t.Fatal(err)
	}
	bad := &workflow.Workflow{Name: "w", Steps: []workflow.Step{
		{ID: "update", Tool: "update", Args: map[string]any{"id": "{{later.result.id}}"}},
	}}
	if err := workflow.ValidateReferences(bad); err == nil || !strings.Contains(err.Error(), "later") {
		t.Fatalf("want unknown root error, got %v", err)
	}
	badCond := &workflow.Workflow{Name: "w", Steps: []workflow.Step{
		{ID: "a", Tool: "t", When: &workflow.Condition{Path: "later.status", Op: "eq", Value: "x"}},
	}}
	if err := workflow.ValidateReferences(badCond); err == nil {
		t.Fatal("condition must reject unknown root")
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	w := &workflow.Workflow{Name: "demo", Steps: []workflow.Step{
		{ID: "a", Tool: "list", Args: map[string]any{"q": "{{input.text}}"}},
		{ID: "b", Tool: "update", Approve: true, When: &workflow.Condition{Path: "input.flag", Op: "eq", Value: "yes"}},
	}}
	raw, err := workflow.Marshal(w)
	if err != nil {
		t.Fatal(err)
	}
	got, err := workflow.Parse(raw)
	if err != nil {
		t.Fatalf("parse marshaled: %v\n%s", err, raw)
	}
	if len(got.Steps) != 2 || got.Steps[1].When == nil || !got.Steps[1].Approve {
		t.Fatalf("round trip=%+v\n%s", got, raw)
	}
}
