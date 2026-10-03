package workflow_test

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/workflow"
)

func TestWorkflowValidateRejectsEmptyName(t *testing.T) {
	w := &workflow.Workflow{Steps: []workflow.Step{{ID: "a", Tool: "x"}}}
	err := w.Validate()
	if err == nil || !strings.Contains(err.Error(), "name") {
		t.Fatalf("err=%v", err)
	}
}

func TestWorkflowValidateRejectsDuplicateStepID(t *testing.T) {
	w := &workflow.Workflow{Name: "n", Steps: []workflow.Step{
		{ID: "a", Tool: "x"}, {ID: "a", Tool: "y"},
	}}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("err=%v", err)
	}
}

func TestWorkflowValidateRejectsEmptyToolOrID(t *testing.T) {
	w := &workflow.Workflow{Name: "n", Steps: []workflow.Step{{ID: "", Tool: "x"}}}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("err=%v", err)
	}
	w2 := &workflow.Workflow{Name: "n", Steps: []workflow.Step{{ID: "a", Tool: ""}}}
	if err := w2.Validate(); err == nil || !strings.Contains(err.Error(), "tool") {
		t.Fatalf("err=%v", err)
	}
}
