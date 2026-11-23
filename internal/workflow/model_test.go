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

func TestWorkflowValidateRejectsRetryOutOfRange(t *testing.T) {
	w := &workflow.Workflow{Name: "n", Steps: []workflow.Step{{ID: "a", Tool: "x", Retry: 6}}}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "retry") {
		t.Fatalf("err=%v", err)
	}
	w.Steps[0].Retry = -1
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "retry") {
		t.Fatalf("negative err=%v", err)
	}
	w.Steps[0].Retry = 5
	if err := w.Validate(); err != nil {
		t.Fatalf("retry=5 err=%v", err)
	}
}

func TestWorkflowValidateRejectsTimeoutOutOfRange(t *testing.T) {
	w := &workflow.Workflow{Name: "n", Steps: []workflow.Step{{ID: "a", Tool: "x", TimeoutMS: 600001}}}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "timeout_ms") {
		t.Fatalf("err=%v", err)
	}
	w.Steps[0].TimeoutMS = -1
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "timeout_ms") {
		t.Fatalf("negative err=%v", err)
	}
	w.Steps[0].TimeoutMS = 600000
	if err := w.Validate(); err != nil {
		t.Fatalf("timeout=600000 err=%v", err)
	}
}

func TestWorkflowValidateRejectsTotalTimeout(t *testing.T) {
	w := &workflow.Workflow{Name: "n", TimeoutMS: 86400001, Steps: []workflow.Step{{ID: "a", Tool: "x"}}}
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "timeout_ms") {
		t.Fatalf("err=%v", err)
	}
	w.TimeoutMS = -1
	if err := w.Validate(); err == nil || !strings.Contains(err.Error(), "timeout_ms") {
		t.Fatalf("negative err=%v", err)
	}
	w.TimeoutMS = 86400000
	if err := w.Validate(); err != nil {
		t.Fatalf("max err=%v", err)
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
