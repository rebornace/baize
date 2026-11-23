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

func TestEvalSampleRequiresAdminAndDraft(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "sample me"})
	if err != nil {
		t.Fatal(err)
	}
	ts := time.Now().UTC().Add(-time.Minute)
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: ts})
	_ = st.AppendEvent(r.ID, store.Event{
		Type: "llm.tool_call", Timestamp: ts.Add(time.Second),
		Data: map[string]any{"name": "list_tickets"},
	})

	body, _ := json.Marshal(map[string]any{
		"from":  "2020-01-01T00:00:00Z",
		"to":    "2030-01-01T00:00:00Z",
		"max_n": 5,
		"mode":  "draft",
		"seed":  "api-test",
	})
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/sample", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer op")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("op: code=%d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/sample", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Created int      `json:"created"`
		CaseIDs []string `json:"case_ids"`
		Scored  int      `json:"scored"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Created < 1 || out.Scored != 0 || len(out.CaseIDs) < 1 {
		t.Fatalf("%+v", out)
	}

	scoreBody, _ := json.Marshal(map[string]any{
		"from":           "2020-01-01T00:00:00Z",
		"to":             "2030-01-01T00:00:00Z",
		"mode":           "score",
		"protocol":       "V0",
		"required_reads": []string{"list_tickets"},
		"consequential":  []string{"create_ticket"},
		"max_n":          5,
		"seed":           "score-seed",
	})
	// new run for score
	r2, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "score sample"})
	_ = st.AppendEvent(r2.ID, store.Event{Type: "run.started", Timestamp: ts})
	_ = st.AppendEvent(r2.ID, store.Event{
		Type: "llm.tool_call", Timestamp: ts.Add(time.Second),
		Data: map[string]any{"name": "list_tickets", "id": "c1"},
	})
	_ = st.AppendEvent(r2.ID, store.Event{
		Type: "tool.result", Timestamp: ts.Add(2 * time.Second),
		Data: map[string]any{"tool_call_id": "c1", "name": "list_tickets", "is_error": false},
	})

	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/sample", bytes.NewReader(scoreBody))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("score: code=%d body=%s", rr.Code, rr.Body.String())
	}

	badScore, _ := json.Marshal(map[string]any{
		"from": "2020-01-01T00:00:00Z",
		"to":   "2030-01-01T00:00:00Z",
		"mode": "score",
	})
	req = httptest.NewRequest(http.MethodPost, "/v0/settings/eval/sample", bytes.NewReader(badScore))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad score: code=%d", rr.Code)
	}
}
