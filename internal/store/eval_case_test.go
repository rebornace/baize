package store

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestListRunsCreatedBetweenAndEvalCaseSnapshot(t *testing.T) {
	runCase := func(t *testing.T, st Store) {
		t.Helper()
		r1, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "one"})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r1.ID, StatusFailed, "", "boom"); err != nil {
			t.Fatal(err)
		}
		if err := st.AppendEvent(r1.ID, Event{Type: "run.started", Timestamp: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
		from := time.Now().Add(-time.Hour)
		to := time.Now().Add(time.Hour)
		runs, err := st.ListRunsCreatedBetween(from, to, StatusFailed, 50)
		if err != nil || len(runs) != 1 || runs[0].ID != r1.ID {
			t.Fatalf("%v %+v", err, runs)
		}
		evs, err := st.ListEvents(r1.ID)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(evs)
		if err != nil {
			t.Fatal(err)
		}
		c := EvalCase{
			ID: "ec1", SourceRunID: r1.ID, Status: EvalCaseDraft,
			Input: r1.Input, EventsJSON: raw, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := st.UpsertEvalCase(c); err != nil {
			t.Fatal(err)
		}
		if err := st.AppendEvent(r1.ID, Event{Type: "llm.message", Timestamp: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
		got, err := st.GetEvalCase("ec1")
		if err != nil {
			t.Fatal(err)
		}
		var snap []Event
		if err := json.Unmarshal(got.EventsJSON, &snap); err != nil {
			t.Fatal(err)
		}
		if len(snap) != 1 {
			t.Fatalf("snapshot must stay at ingest size, got %d", len(snap))
		}
	}

	t.Run("memory", func(t *testing.T) {
		runCase(t, NewMemory())
	})
	t.Run("sqlite", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "eval_case.db")
		st, err := OpenSQLite(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.Close() })
		runCase(t, st)
	})
}

func TestListRunsCreatedBetweenLimit(t *testing.T) {
	st := NewMemory()
	for i := 0; i < 3; i++ {
		if _, err := st.CreateRun(CreateRunInput{AgentID: "a", Input: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	runs, err := st.ListRunsCreatedBetween(from, to, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 {
		t.Fatalf("limit 0 => default 100, got %d runs", len(runs))
	}
}

func TestListRunsCreatedBetweenStatusFilter(t *testing.T) {
	st := NewMemory()
	rOk, _ := st.CreateRun(CreateRunInput{AgentID: "a", Input: "ok"})
	_ = st.UpdateRun(rOk.ID, StatusSucceeded, "done", "")
	rFail, _ := st.CreateRun(CreateRunInput{AgentID: "a", Input: "bad"})
	_ = st.UpdateRun(rFail.ID, StatusFailed, "", "err")
	from := time.Now().Add(-time.Hour)
	to := time.Now().Add(time.Hour)
	runs, err := st.ListRunsCreatedBetween(from, to, StatusFailed, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].ID != rFail.ID {
		t.Fatalf("want failed only, got %+v", runs)
	}
}

func TestEvalCaseCRUDMemory(t *testing.T) {
	st := NewMemory()
	c := EvalCase{
		ID: "ec1", Status: EvalCaseConfirmed, Protocol: "V1",
		Input: "hi", EventsJSON: []byte(`[]`),
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	if err := st.UpsertEvalCase(c); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetEvalCase("ec1")
	if err != nil || got.Protocol != "V1" {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	list, err := st.ListEvalCases(10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %+v err=%v", list, err)
	}
	if _, ok, err := st.FindEvalCaseBySourceRun("missing"); err != nil || ok {
		t.Fatalf("find missing: ok=%v err=%v", ok, err)
	}
	c.SourceRunID = "run_x"
	if err := st.UpsertEvalCase(c); err != nil {
		t.Fatal(err)
	}
	if found, ok, err := st.FindEvalCaseBySourceRun("run_x"); err != nil || !ok || found.ID != "ec1" {
		t.Fatalf("find: %+v ok=%v err=%v", found, ok, err)
	}
	if err := st.DeleteEvalCase("ec1"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.GetEvalCase("ec1"); err == nil {
		t.Fatal("expected not found after delete")
	}
}
