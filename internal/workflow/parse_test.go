package workflow_test

import (
	"testing"

	"github.com/rebornace/baize/internal/workflow"
)

const sample = `
name: ticket-triage
steps:
  - id: fetch
    tool: search_tickets
    args:
      query: "{{input.text}}"
  - id: reply
    tool: reply_ticket
    approve: true
    args:
      text: "{{fetch.result.summary}}"
`

func TestParseOK(t *testing.T) {
	w, err := workflow.Parse([]byte(sample))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if w.Name != "ticket-triage" || len(w.Steps) != 2 {
		t.Fatalf("w=%+v", w)
	}
	if !w.Steps[1].Approve {
		t.Fatalf("approve=%v", w.Steps[1].Approve)
	}
	if w.Steps[0].Args["query"] != "{{input.text}}" {
		t.Fatalf("args=%v", w.Steps[0].Args)
	}
}

func TestParseRejectsUnknownField(t *testing.T) {
	bad := "name: n\nsteps:\n  - id: a\n    tool: x\n    when: \"{{a.result}}\"\n"
	if _, err := workflow.Parse([]byte(bad)); err == nil {
		t.Fatal("want unknown-field error")
	}
}

func TestParseRequiresWorkflowKey(t *testing.T) {
	if _, err := workflow.Parse([]byte("name: n\n")); err == nil {
		t.Fatal("want steps required")
	}
}
