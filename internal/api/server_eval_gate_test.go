package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eval"
	"github.com/rebornace/baize/internal/store"
)

func TestEvalGateRunAndThreshold(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	now := time.Now().UTC()
	mk := func(id string, status store.EvalCaseStatus) {
		t.Helper()
		c := store.EvalCase{
			ID: id, Status: status, Protocol: "V1",
			EventsJSON: []byte("[]"), CreatedAt: now, UpdatedAt: now,
		}
		if status == store.EvalCaseConfirmed {
			c.RequiredReads = []string{"read_ticket"}
			c.Consequential = []string{"update_ticket"}
		}
		if err := st.UpsertEvalCase(c); err != nil {
			t.Fatal(err)
		}
	}
	mk("ec_gate_fail", store.EvalCaseConfirmed)
	passEvents := []store.Event{
		{Type: "run.started", Timestamp: now, Data: map[string]any{}},
		{Type: "llm.tool_call", Timestamp: now, Data: map[string]any{"id": "c1", "name": "list_tickets", "arguments": map[string]any{}}},
		{Type: "tool.result", Timestamp: now, Data: map[string]any{"id": "c1", "name": "list_tickets", "is_error": false, "content": map[string]any{"tickets": []any{}}}},
		{Type: "llm.message", Timestamp: now, Data: map[string]any{"role": "assistant", "content": "ok"}},
	}
	passJSON, _ := json.Marshal(passEvents)
	if err := st.UpsertEvalCase(store.EvalCase{
		ID: "ec_gate_pass", Status: store.EvalCaseConfirmed, Protocol: "V0",
		RequiredReads: []string{"list_tickets"}, Consequential: []string{"create_ticket"},
		EventsJSON: passJSON, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertEvalCase(store.EvalCase{
		ID: "ec_gate_draft", Status: store.EvalCaseDraft, Protocol: "V1",
		EventsJSON: []byte("[]"), CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}

	h := srv.Handler()
	run := func() eval.GateResult {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/gate/run", nil)
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("run=%d %s", rr.Code, rr.Body.String())
		}
		var res eval.GateResult
		if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
			t.Fatal(err)
		}
		return res
	}

	first := run()
	if first.Total != 2 || first.Passed != 1 || first.Failed != 1 {
		t.Fatalf("first=%+v", first)
	}
	if first.PassRate != 0.5 || first.MinPassRate != 0.9 || first.Pass {
		t.Fatalf("gate=%+v", first)
	}
	if len(first.Failures) != 1 || first.Failures[0].ID != "ec_gate_fail" {
		t.Fatalf("failures=%+v", first.Failures)
	}
	if len(first.Failures[0].Codes) == 0 || first.Failures[0].Notes == "" {
		t.Fatalf("failure detail=%+v", first.Failures[0])
	}

	put := httptest.NewRequest(http.MethodPut, "/v0/settings/eval/gate",
		bytes.NewBufferString(`{"min_pass_rate":0.4,"score_history_cap":120}`))
	put.Header.Set("Authorization", "Bearer adm")
	pr := httptest.NewRecorder()
	h.ServeHTTP(pr, put)
	if pr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", pr.Code, pr.Body.String())
	}
	second := run()
	if !second.Pass || second.MinPassRate != 0.4 {
		t.Fatalf("second=%+v", second)
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/gate", nil)
	get.Header.Set("Authorization", "Bearer adm")
	gr := httptest.NewRecorder()
	h.ServeHTTP(gr, get)
	var state struct {
		MinPassRate     float64          `json:"min_pass_rate"`
		ScoreHistoryCap int              `json:"score_history_cap"`
		Last            *eval.GateResult `json:"last"`
	}
	if err := json.Unmarshal(gr.Body.Bytes(), &state); err != nil {
		t.Fatal(err)
	}
	if state.MinPassRate != 0.4 || state.ScoreHistoryCap != 120 || state.Last == nil ||
		state.Last.Total != 2 || !state.Last.Pass {
		t.Fatalf("state=%+v", state)
	}

	bad := httptest.NewRequest(http.MethodPut, "/v0/settings/eval/gate",
		bytes.NewBufferString(`{"min_pass_rate":1.5}`))
	bad.Header.Set("Authorization", "Bearer adm")
	br := httptest.NewRecorder()
	h.ServeHTTP(br, bad)
	if br.Code != http.StatusBadRequest {
		t.Fatalf("bad threshold=%d", br.Code)
	}

	badCap := httptest.NewRequest(http.MethodPut, "/v0/settings/eval/gate",
		bytes.NewBufferString(`{"min_pass_rate":0.4,"score_history_cap":5}`))
	badCap.Header.Set("Authorization", "Bearer adm")
	cr := httptest.NewRecorder()
	h.ServeHTTP(cr, badCap)
	if cr.Code != http.StatusBadRequest {
		t.Fatalf("bad cap=%d", cr.Code)
	}
}

func TestEvalGateEmptyIsNotPass(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/gate/run", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("run=%d", rr.Code)
	}
	var res eval.GateResult
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	if !res.Empty || res.Pass || res.Total != 0 {
		t.Fatalf("empty=%+v", res)
	}
}
