package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestEvalRunsRequiresAdmin(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	q := "?from=2026-01-01T00:00:00Z&to=2026-12-31T00:00:00Z"

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs"+q, nil)
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator: code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs"+q, nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/settings/eval/runs", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing window: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestEvalIngestScoreExport(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()

	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a",
		Input:   "只查询工单列表，了解当前有哪些工单。不要新建工单，也不要改任何工单状态。",
	})
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UTC().Add(-time.Minute)
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: ts})
	_ = st.AppendEvent(r.ID, store.Event{
		Type:      "llm.tool_call",
		Timestamp: ts.Add(time.Second),
		Data:      map[string]any{"id": "call_1", "name": "list_tickets", "arguments": map[string]any{}},
	})
	_ = st.AppendEvent(r.ID, store.Event{
		Type:      "tool.result",
		Timestamp: ts.Add(2 * time.Second),
		Data:      map[string]any{"tool_call_id": "call_1", "name": "list_tickets", "is_error": false},
	})

	ingestBody, _ := json.Marshal(map[string]any{
		"run_id":         r.ID,
		"protocol":       "V0",
		"required_reads": []string{"list_tickets"},
		"consequential":  []string{"create_ticket", "update_ticket_status"},
	})
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases", bytes.NewReader(ingestBody))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("ingest: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID     string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Status != string(store.EvalCaseConfirmed) {
		t.Fatalf("status=%q", created.Status)
	}

	scoreReq := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases/"+created.ID+"/score", nil)
	scoreReq.Header.Set("Authorization", "Bearer adm")
	scoreRR := httptest.NewRecorder()
	h.ServeHTTP(scoreRR, scoreReq)
	if scoreRR.Code != http.StatusOK {
		t.Fatalf("score confirmed: code=%d body=%s", scoreRR.Code, scoreRR.Body.String())
	}
	var scoreOut struct {
		Pass  bool            `json:"pass"`
		Codes json.RawMessage `json:"codes"`
	}
	if err := json.Unmarshal(scoreRR.Body.Bytes(), &scoreOut); err != nil {
		t.Fatal(err)
	}
	if !scoreOut.Pass {
		t.Fatalf("score: %s", scoreRR.Body.String())
	}
	if string(scoreOut.Codes) != "[]" {
		t.Fatalf("pass score must serialize codes as [], got %s", scoreOut.Codes)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/cases/"+created.ID+"/export", nil)
	exportReq.Header.Set("Authorization", "Bearer adm")
	exportRR := httptest.NewRecorder()
	h.ServeHTTP(exportRR, exportReq)
	if exportRR.Code != http.StatusOK {
		t.Fatalf("export: code=%d body=%s", exportRR.Code, exportRR.Body.String())
	}
	var exportDoc map[string]any
	if err := json.Unmarshal(exportRR.Body.Bytes(), &exportDoc); err != nil {
		t.Fatal(err)
	}
	if exportDoc["case"] == nil || exportDoc["events"] == nil {
		t.Fatalf("export shape: %v", exportDoc)
	}
}

func TestEvalDraftScoreRejected(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()

	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: time.Now().UTC()})

	body, _ := json.Marshal(map[string]any{"run_id": r.ID})
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("ingest draft: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &created)

	scoreReq := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases/"+created.ID+"/score", nil)
	scoreReq.Header.Set("Authorization", "Bearer adm")
	scoreRR := httptest.NewRecorder()
	h.ServeHTTP(scoreRR, scoreReq)
	if scoreRR.Code != http.StatusBadRequest {
		t.Fatalf("draft score: code=%d body=%s", scoreRR.Code, scoreRR.Body.String())
	}
}

func TestEvalIngestConflict(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()

	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	body, _ := json.Marshal(map[string]any{"run_id": r.ID})

	post := func() int {
		req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer adm")
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr.Code
	}
	if post() != http.StatusCreated {
		t.Fatal("first ingest failed")
	}
	if post() != http.StatusConflict {
		t.Fatal("want 409 on duplicate ingest")
	}
}
