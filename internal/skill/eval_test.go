package skill_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/rebornace/baize/internal/eval/evidence"
	"github.com/rebornace/baize/internal/skill"
)

func TestExampleTicketTriageEVAL(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "skills", "ticket-triage", "EVAL.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := skill.ParseEvalYAML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Cases) < 2 {
		t.Fatalf("expected at least 2 cases, got %d", len(doc.Cases))
	}
	if doc.Cases[0].ID != "EV-V0-list-only" {
		t.Fatalf("first case: %+v", doc.Cases[0])
	}
}

func TestParseEvalYAMLRoundTrip(t *testing.T) {
	raw := []byte(`harness: evidence-eval
cases:
  - id: c1
    protocol: V0
    input: "只列表"
    required_reads: ["list_tickets"]
    consequential: ["create_ticket"]
`)
	doc, err := skill.ParseEvalYAML(raw)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Harness != "evidence-eval" || len(doc.Cases) != 1 || doc.Cases[0].ID != "c1" {
		t.Fatalf("%+v", doc)
	}
	out, err := skill.MarshalEvalYAML(doc)
	if err != nil {
		t.Fatal(err)
	}
	doc2, err := skill.ParseEvalYAML(out)
	if err != nil || doc2.Cases[0].Input != "只列表" {
		t.Fatalf("%v %+v", err, doc2)
	}
}

func TestParseEvalYAMLRejects(t *testing.T) {
	if _, err := skill.ParseEvalYAML([]byte("harness: other\ncases: []\n")); err == nil {
		t.Fatal("bad harness")
	}
	if _, err := skill.ParseEvalYAML([]byte("harness: evidence-eval\ncases:\n  - id: x\n    protocol: V0\n    input: hi\n    extra: 1\n")); err == nil {
		t.Fatal("unknown field")
	}
	if _, err := skill.ParseEvalYAML([]byte("harness: evidence-eval\ncases:\n  - id: \"\"\n    protocol: V0\n    input: hi\n")); err == nil {
		t.Fatal("empty id")
	}
	if _, err := skill.ParseEvalYAML([]byte("harness: evidence-eval\ncases:\n  - id: a\n    protocol: V0\n    input: hi\n  - id: a\n    protocol: V1\n    input: there\n")); err == nil {
		t.Fatal("duplicate id")
	}
	if _, err := skill.ParseEvalYAML([]byte("harness: evidence-eval\ncases:\n  - id: x\n    protocol: V9\n    input: hi\n")); err == nil {
		t.Fatal("bad protocol")
	}
	if _, err := skill.ParseEvalYAML([]byte("harness: evidence-eval\ncases:\n  - id: x\n    protocol: V0\n    input: \"\"\n")); err == nil {
		t.Fatal("empty input")
	}
}

func TestToEvidenceCase(t *testing.T) {
	in := skill.EvalCase{
		ID:                     "c1",
		Protocol:               "V2",
		Input:                  "prompt must not leak",
		Notes:                  "notes must not leak",
		RequiredReads:          []string{"list_tickets"},
		Consequential:          []string{"create_ticket"},
		ExpectConsequentialAny: []string{"update_ticket"},
		Chain: []skill.EvalChainLink{
			{After: "list_tickets", Before: "create_ticket"},
		},
		ForbidTools: []string{"delete_all"},
		ArgContains: []skill.EvalArgContains{
			{Tool: "create_ticket", Substr: "urgent"},
		},
		ExpectState: []skill.EvalExpectState{
			{ID: "t1", TitleContains: "foo", Status: "open"},
		},
	}
	got := skill.ToEvidenceCase(in)
	want := evidence.Case{
		ID:                     "c1",
		Protocol:               "V2",
		RequiredReads:          []string{"list_tickets"},
		Consequential:          []string{"create_ticket"},
		ExpectConsequentialAny: []string{"update_ticket"},
		Chain: []evidence.ChainLink{
			{After: "list_tickets", Before: "create_ticket"},
		},
		ForbidTools: []string{"delete_all"},
		ArgContains: []evidence.ArgContains{
			{Tool: "create_ticket", Substr: "urgent"},
		},
		ExpectState: []evidence.ExpectTicket{
			{ID: "t1", TitleContains: "foo", Status: "open"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mapping mismatch:\n got: %+v\nwant: %+v", got, want)
	}
	ev := reflect.ValueOf(got)
	for _, field := range []string{"Input", "Notes"} {
		if ev.FieldByName(field).IsValid() {
			t.Fatalf("evidence.Case must not have field %q", field)
		}
	}
}
