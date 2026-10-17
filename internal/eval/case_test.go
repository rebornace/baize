package eval_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

func evidenceEvalDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "scripts", "evidence-eval"))
}

func loadFixtureEvents(t *testing.T, name string) []store.Event {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(evidenceEvalDir(t), "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	var fixture []struct {
		Type string         `json:"type"`
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UTC().Add(-time.Minute)
	out := make([]store.Event, 0, len(fixture))
	for _, f := range fixture {
		out = append(out, store.Event{Type: f.Type, Timestamp: ts, Data: f.Data})
		ts = ts.Add(time.Second)
	}
	return out
}

func TestIngestDraftAndRejectScore(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: time.Now().UTC()})
	c, err := eval.IngestFromRun(st, eval.IngestInput{RunID: r.ID})
	if err != nil || c.Status != store.EvalCaseDraft {
		t.Fatalf("%+v %v", c, err)
	}
	_, err = eval.ScoreCase(st, c.ID)
	if err == nil {
		t.Fatal("draft must not score")
	}
}

func TestIngestProtocolOnlyStaysDraft(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	c, err := eval.IngestFromRun(st, eval.IngestInput{RunID: r.ID, Protocol: "V0"})
	if err != nil || c.Status != store.EvalCaseDraft {
		t.Fatalf("want draft: %+v %v", c, err)
	}
	_, err = eval.ScoreCase(st, c.ID)
	if err != eval.ErrNotScorable {
		t.Fatalf("want ErrNotScorable, got %v", err)
	}
}

func TestIngestMissingConsequentialStaysDraft(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	c, err := eval.IngestFromRun(st, eval.IngestInput{
		RunID:         r.ID,
		Protocol:      "V0",
		RequiredReads: []string{"list_tickets"},
	})
	if err != nil || c.Status != store.EvalCaseDraft {
		t.Fatalf("want draft: %+v %v", c, err)
	}
}

func TestIngestEmptySlicesCanConfirm(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	c, err := eval.IngestFromRun(st, eval.IngestInput{
		RunID:         r.ID,
		Protocol:      "V0",
		RequiredReads: []string{},
		Consequential: []string{},
	})
	if err != nil || c.Status != store.EvalCaseConfirmed {
		t.Fatalf("empty slices are provided: %+v %v", c, err)
	}
}

func TestPatchConfirmRejectsIncomplete(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	c, _ := eval.IngestFromRun(st, eval.IngestInput{RunID: r.ID, Protocol: "V0"})
	_, err := eval.PatchCase(st, c.ID, eval.PatchInput{
		SetStatus: true,
		Status:    store.EvalCaseConfirmed,
	})
	if err != eval.ErrNotScorable {
		t.Fatalf("want ErrNotScorable, got %v", err)
	}
}

func TestPatchCompletesExpectationsThenScores(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	for _, ev := range loadFixtureEvents(t, "EV-V0-list-only.pass.events.json") {
		_ = st.AppendEvent(r.ID, ev)
	}
	c, _ := eval.IngestFromRun(st, eval.IngestInput{RunID: r.ID, Protocol: "V0"})
	c, err := eval.PatchCase(st, c.ID, eval.PatchInput{
		SetRequiredReads: true,
		RequiredReads:    []string{"list_tickets"},
		SetConsequential: true,
		Consequential:    []string{"create_ticket", "update_ticket_status"},
	})
	if err != nil || c.Status != store.EvalCaseConfirmed {
		t.Fatalf("patch promote: %+v %v", c, err)
	}
	if _, err := eval.ScoreCase(st, c.ID); err != nil {
		t.Fatalf("score after patch: %v", err)
	}
}

func TestIngestConflict(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	_, err := eval.IngestFromRun(st, eval.IngestInput{RunID: r.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = eval.IngestFromRun(st, eval.IngestInput{RunID: r.ID})
	if err != eval.ErrConflict {
		t.Fatalf("want ErrConflict, got %v", err)
	}
}

func TestIngestReplaceKeepsID(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: time.Now().UTC()})
	c1, _ := eval.IngestFromRun(st, eval.IngestInput{RunID: r.ID})
	_ = st.AppendEvent(r.ID, store.Event{Type: "llm.message", Timestamp: time.Now().UTC()})
	c2, err := eval.IngestFromRun(st, eval.IngestInput{RunID: r.ID, Replace: true})
	if err != nil || c2.ID != c1.ID {
		t.Fatalf("want same id: %v %s %s", err, c1.ID, c2.ID)
	}
	var snap []store.Event
	_ = json.Unmarshal(c2.EventsJSON, &snap)
	if len(snap) != 2 {
		t.Fatalf("replaced snapshot size=%d", len(snap))
	}
	if c2.LastScorePass != nil || c2.LastScoredAt != nil {
		t.Fatal("replace must clear last score fields")
	}
}

func TestConfirmAndScore(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{
		AgentID: "a",
		Input:   "只查询工单列表，了解当前有哪些工单。不要新建工单，也不要改任何工单状态。",
	})
	for _, ev := range loadFixtureEvents(t, "EV-V0-list-only.pass.events.json") {
		if err := st.AppendEvent(r.ID, ev); err != nil {
			t.Fatal(err)
		}
	}
	c, err := eval.IngestFromRun(st, eval.IngestInput{
		RunID:         r.ID,
		Protocol:      "V0",
		RequiredReads: []string{"list_tickets"},
		Consequential: []string{"create_ticket", "update_ticket_status"},
	})
	if err != nil || c.Status != store.EvalCaseConfirmed {
		t.Fatalf("ingest: %+v %v", c, err)
	}
	res, err := eval.ScoreCase(st, c.ID)
	if err != nil || !res.Pass {
		t.Fatalf("%v %+v", err, res)
	}
	got, err := st.GetEvalCase(c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.LastScorePass == nil || !*got.LastScorePass {
		t.Fatalf("score not persisted: %+v", got)
	}
}

func TestExportNormalizesToolResultID(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hello"})
	ts := time.Now().UTC()
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: ts})
	_ = st.AppendEvent(r.ID, store.Event{
		Type:      "llm.tool_call",
		Timestamp: ts.Add(time.Second),
		Data:      map[string]any{"id": "call_1", "name": "list_tickets"},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type:      "tool.result",
		Timestamp: ts.Add(2 * time.Second),
		Data:      map[string]any{"tool_call_id": "call_1", "name": "list_tickets", "is_error": false},
	})
	c, err := eval.IngestFromRun(st, eval.IngestInput{
		RunID:         r.ID,
		Protocol:      "V0",
		RequiredReads: []string{"list_tickets"},
		Consequential: []string{"create_ticket"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := eval.Export(st, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Events []struct {
			Type string         `json:"type"`
			Data map[string]any `json:"data"`
		} `json:"events"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	for _, ev := range doc.Events {
		if ev.Type == "tool.result" {
			result = ev.Data
			break
		}
	}
	if result == nil {
		t.Fatal("missing tool.result")
	}
	if result["id"] != "call_1" || result["tool_call_id"] != "call_1" {
		t.Fatalf("export normalize: %v", result)
	}
}

func TestExportShape(t *testing.T) {
	st := store.NewMemory()
	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hello"})
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: time.Now().UTC()})
	c, err := eval.IngestFromRun(st, eval.IngestInput{
		RunID:         r.ID,
		Protocol:      "V0",
		RequiredReads: []string{"list_tickets"},
		Consequential: []string{"create_ticket"},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := eval.Export(st, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	caseObj, ok := doc["case"].(map[string]any)
	if !ok {
		t.Fatalf("missing case: %v", doc)
	}
	if caseObj["protocol"] != "V0" || caseObj["source_run_id"] != r.ID {
		t.Fatalf("case fields: %v", caseObj)
	}
	events, ok := doc["events"].([]any)
	if !ok || len(events) != 1 {
		t.Fatalf("events: %v", doc["events"])
	}
}
