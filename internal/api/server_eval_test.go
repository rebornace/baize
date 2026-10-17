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

func TestEvalIngestTrajectoryFields(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: time.Now().UTC()})

	ingestBody, _ := json.Marshal(map[string]any{
		"run_id":         r.ID,
		"protocol":       "V0",
		"required_reads": []string{"list_tickets"},
		"consequential":  []string{"create_ticket"},
		"forbid_tools":   []string{"create_ticket"},
		"arg_contains": []map[string]string{
			{"tool": "list_tickets", "substr": "page"},
		},
		"expect_state": []map[string]string{
			{"id": "t1", "title_contains": "bug", "status": "open"},
		},
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
		ID          string                  `json:"id"`
		ForbidTools []string                `json:"forbid_tools"`
		ArgContains []store.EvalArgContains `json:"arg_contains"`
		ExpectState []store.EvalExpectState `json:"expect_state"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if len(created.ForbidTools) != 1 || created.ForbidTools[0] != "create_ticket" {
		t.Fatalf("forbid_tools: %+v", created.ForbidTools)
	}
	if len(created.ArgContains) != 1 || created.ArgContains[0].Tool != "list_tickets" || created.ArgContains[0].Substr != "page" {
		t.Fatalf("arg_contains: %+v", created.ArgContains)
	}
	if len(created.ExpectState) != 1 || created.ExpectState[0].ID != "t1" || created.ExpectState[0].TitleContains != "bug" || created.ExpectState[0].Status != "open" {
		t.Fatalf("expect_state: %+v", created.ExpectState)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/cases/"+created.ID, nil)
	getReq.Header.Set("Authorization", "Bearer adm")
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("get: code=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var fetched struct {
		ForbidTools []string                `json:"forbid_tools"`
		ArgContains []store.EvalArgContains `json:"arg_contains"`
		ExpectState []store.EvalExpectState `json:"expect_state"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &fetched); err != nil {
		t.Fatal(err)
	}
	if len(fetched.ForbidTools) != 1 || fetched.ForbidTools[0] != "create_ticket" {
		t.Fatalf("GET forbid_tools: %+v", fetched.ForbidTools)
	}
	if len(fetched.ArgContains) != 1 || fetched.ArgContains[0].Tool != "list_tickets" {
		t.Fatalf("GET arg_contains: %+v", fetched.ArgContains)
	}
	if len(fetched.ExpectState) != 1 || fetched.ExpectState[0].ID != "t1" {
		t.Fatalf("GET expect_state: %+v", fetched.ExpectState)
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

func evalScoredCase(t *testing.T, srv *Server) string {
	t.Helper()
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
		ID string `json:"id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	scoreReq := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases/"+created.ID+"/score", nil)
	scoreReq.Header.Set("Authorization", "Bearer adm")
	scoreRR := httptest.NewRecorder()
	h.ServeHTTP(scoreRR, scoreReq)
	if scoreRR.Code != http.StatusOK {
		t.Fatalf("score: code=%d body=%s", scoreRR.Code, scoreRR.Body.String())
	}
	return created.ID
}

func TestEvalCalibrationAdminAndSummary(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	caseID := evalScoredCase(t, srv)

	body, _ := json.Marshal(map[string]any{"agree": true, "note": "looks good"})
	req := httptest.NewRequest(http.MethodPatch, "/v0/settings/eval/cases/"+caseID+"/calibration", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer adm")
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("calibrate: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		HumanAgree   *bool  `json:"human_agree"`
		HumanNote    string `json:"human_note"`
		CalibratedAt string `json:"calibrated_at"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.HumanAgree == nil || !*out.HumanAgree || out.HumanNote != "looks good" || out.CalibratedAt == "" {
		t.Fatalf("response: %+v", out)
	}

	sumReq := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/calibration-summary", nil)
	sumReq.Header.Set("Authorization", "Bearer adm")
	sumRR := httptest.NewRecorder()
	h.ServeHTTP(sumRR, sumReq)
	if sumRR.Code != http.StatusOK {
		t.Fatalf("summary: code=%d body=%s", sumRR.Code, sumRR.Body.String())
	}
	var sum struct {
		ScoredCount     int      `json:"scored_count"`
		CalibratedCount int      `json:"calibrated_count"`
		AgreementRate   *float64 `json:"agreement_rate"`
	}
	if err := json.Unmarshal(sumRR.Body.Bytes(), &sum); err != nil {
		t.Fatal(err)
	}
	if sum.ScoredCount < 1 || sum.CalibratedCount != 1 || sum.AgreementRate == nil || *sum.AgreementRate != 1 {
		t.Fatalf("summary: %s", sumRR.Body.String())
	}
}

func TestEvalCalibrationOperatorForbidden(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	caseID := evalScoredCase(t, srv)
	body, _ := json.Marshal(map[string]any{"agree": true})

	patchReq := httptest.NewRequest(http.MethodPatch, "/v0/settings/eval/cases/"+caseID+"/calibration", bytes.NewReader(body))
	patchReq.Header.Set("Authorization", "Bearer op")
	patchReq.Header.Set("Content-Type", "application/json")
	patchRR := httptest.NewRecorder()
	h.ServeHTTP(patchRR, patchReq)
	if patchRR.Code != http.StatusForbidden {
		t.Fatalf("operator patch: code=%d body=%s", patchRR.Code, patchRR.Body.String())
	}

	sumReq := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/calibration-summary", nil)
	sumReq.Header.Set("Authorization", "Bearer op")
	sumRR := httptest.NewRecorder()
	h.ServeHTTP(sumRR, sumReq)
	if sumRR.Code != http.StatusForbidden {
		t.Fatalf("operator summary: code=%d body=%s", sumRR.Code, sumRR.Body.String())
	}
}

func TestEvalCalibrationNotScoredRejected(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()

	r, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: time.Now().UTC()})
	ingestBody, _ := json.Marshal(map[string]any{"run_id": r.ID})
	req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases", bytes.NewReader(ingestBody))
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

	body, _ := json.Marshal(map[string]any{"agree": true})
	patchReq := httptest.NewRequest(http.MethodPatch, "/v0/settings/eval/cases/"+created.ID+"/calibration", bytes.NewReader(body))
	patchReq.Header.Set("Authorization", "Bearer adm")
	patchReq.Header.Set("Content-Type", "application/json")
	patchRR := httptest.NewRecorder()
	h.ServeHTTP(patchRR, patchReq)
	if patchRR.Code != http.StatusBadRequest {
		t.Fatalf("draft calibrate: code=%d body=%s", patchRR.Code, patchRR.Body.String())
	}
}

func TestEvalCalibrationSummaryNullRate(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	_ = evalScoredCase(t, srv)

	sumReq := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/calibration-summary", nil)
	sumReq.Header.Set("Authorization", "Bearer adm")
	sumRR := httptest.NewRecorder()
	h.ServeHTTP(sumRR, sumReq)
	if sumRR.Code != http.StatusOK {
		t.Fatalf("summary: code=%d body=%s", sumRR.Code, sumRR.Body.String())
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(sumRR.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if string(raw["agreement_rate"]) != "null" {
		t.Fatalf("want agreement_rate null, got %s", raw["agreement_rate"])
	}
}

func TestEvalPatchTrajectoryFieldsHTTP(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()

	r, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(r.ID, store.Event{Type: "run.started", Timestamp: time.Now().UTC()})
	ingestBody, _ := json.Marshal(map[string]any{"run_id": r.ID})
	ingestReq := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases", bytes.NewReader(ingestBody))
	ingestReq.Header.Set("Authorization", "Bearer adm")
	ingestReq.Header.Set("Content-Type", "application/json")
	ingestRR := httptest.NewRecorder()
	h.ServeHTTP(ingestRR, ingestReq)
	if ingestRR.Code != http.StatusCreated {
		t.Fatalf("ingest: code=%d body=%s", ingestRR.Code, ingestRR.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(ingestRR.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	patchBody, _ := json.Marshal(map[string]any{
		"forbid_tools": []string{"create_ticket"},
		"arg_contains": []map[string]string{
			{"tool": "list_tickets", "substr": "page"},
		},
		"expect_state": []map[string]string{
			{"id": "t1", "title_contains": "bug", "status": "open"},
		},
	})
	patchReq := httptest.NewRequest(http.MethodPatch, "/v0/settings/eval/cases/"+created.ID, bytes.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer adm")
	patchReq.Header.Set("Content-Type", "application/json")
	patchRR := httptest.NewRecorder()
	h.ServeHTTP(patchRR, patchReq)
	if patchRR.Code != http.StatusOK {
		t.Fatalf("patch: code=%d body=%s", patchRR.Code, patchRR.Body.String())
	}
	var out struct {
		ForbidTools []string                `json:"forbid_tools"`
		ArgContains []store.EvalArgContains `json:"arg_contains"`
		ExpectState []store.EvalExpectState `json:"expect_state"`
	}
	if err := json.Unmarshal(patchRR.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.ForbidTools) != 1 || out.ForbidTools[0] != "create_ticket" {
		t.Fatalf("forbid_tools: %+v", out.ForbidTools)
	}
	if len(out.ArgContains) != 1 || out.ArgContains[0].Tool != "list_tickets" || out.ArgContains[0].Substr != "page" {
		t.Fatalf("arg_contains: %+v", out.ArgContains)
	}
	if len(out.ExpectState) != 1 || out.ExpectState[0].ID != "t1" || out.ExpectState[0].TitleContains != "bug" || out.ExpectState[0].Status != "open" {
		t.Fatalf("expect_state: %+v", out.ExpectState)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(patchRR.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["last_score_codes"]; ok {
		t.Fatalf("unscored case must omit last_score_codes, got %s", patchRR.Body.String())
	}
}

func TestEvalCheckStateAdminSnapshotAndACL(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	h := srv.Handler()
	caseID := evalScoredCase(t, srv)

	c, err := eval.PatchCase(st, caseID, eval.PatchInput{
		SetExpectState: true,
		ExpectState: []store.EvalExpectState{
			{ID: "t1", TitleContains: "链路评测", Status: "closed"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	agree := true
	c.HumanAgree = &agree
	c.HumanNote = "calibrated"
	cal := time.Now().UTC()
	c.CalibratedAt = &cal
	if err := st.UpsertEvalCase(c); err != nil {
		t.Fatal(err)
	}

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tickets" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":"t1","title":"链路评测","status":"closed"}]`))
	}))
	defer mock.Close()

	postCheck := func(token, mockURL string) *httptest.ResponseRecorder {
		t.Helper()
		body, _ := json.Marshal(map[string]string{"mock_base_url": mockURL})
		req := httptest.NewRequest(http.MethodPost, "/v0/settings/eval/cases/"+caseID+"/check-state", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		return rr
	}

	opRR := postCheck("op", mock.URL)
	if opRR.Code != http.StatusForbidden {
		t.Fatalf("operator check-state: code=%d body=%s", opRR.Code, opRR.Body.String())
	}

	badRR := postCheck("adm", "  ")
	if badRR.Code != http.StatusBadRequest {
		t.Fatalf("missing url: code=%d body=%s", badRR.Code, badRR.Body.String())
	}

	okRR := postCheck("adm", mock.URL)
	if okRR.Code != http.StatusOK {
		t.Fatalf("check-state: code=%d body=%s", okRR.Code, okRR.Body.String())
	}
	var checked struct {
		HasStateSnapshot bool            `json:"has_state_snapshot"`
		StateSnapshot    json.RawMessage `json:"state_snapshot"`
		LastScorePass    *bool           `json:"last_score_pass"`
		HumanAgree       *bool           `json:"human_agree"`
		HumanNote        string          `json:"human_note"`
		CalibratedAt     string          `json:"calibrated_at"`
	}
	if err := json.Unmarshal(okRR.Body.Bytes(), &checked); err != nil {
		t.Fatal(err)
	}
	if !checked.HasStateSnapshot || len(checked.StateSnapshot) == 0 {
		t.Fatalf("want state_snapshot in response: %s", okRR.Body.String())
	}
	if checked.LastScorePass == nil || !*checked.LastScorePass {
		t.Fatalf("want pass after check-state: %s", okRR.Body.String())
	}
	if checked.HumanAgree == nil || !*checked.HumanAgree || checked.HumanNote != "calibrated" || checked.CalibratedAt == "" {
		t.Fatalf("calibration must survive check-state: %+v", checked)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/cases", nil)
	listReq.Header.Set("Authorization", "Bearer adm")
	listRR := httptest.NewRecorder()
	h.ServeHTTP(listRR, listReq)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list: code=%d body=%s", listRR.Code, listRR.Body.String())
	}
	var list struct {
		Items []struct {
			ID               string          `json:"id"`
			HasStateSnapshot bool            `json:"has_state_snapshot"`
			StateSnapshot    json.RawMessage `json:"state_snapshot"`
		} `json:"items"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	var row *struct {
		ID               string          `json:"id"`
		HasStateSnapshot bool            `json:"has_state_snapshot"`
		StateSnapshot    json.RawMessage `json:"state_snapshot"`
	}
	for i := range list.Items {
		if list.Items[i].ID == caseID {
			row = &list.Items[i]
			break
		}
	}
	if row == nil {
		t.Fatal("case missing from list")
	}
	if !row.HasStateSnapshot || row.StateSnapshot != nil {
		t.Fatalf("list must expose has_state_snapshot only: %+v", row)
	}
}

func TestEvalCalibrationNoteOverwriteAndClear(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	caseID := evalScoredCase(t, srv)

	set := func(body map[string]any) {
		t.Helper()
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPatch, "/v0/settings/eval/cases/"+caseID+"/calibration", bytes.NewReader(b))
		req.Header.Set("Authorization", "Bearer adm")
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("patch: code=%d body=%s", rr.Code, rr.Body.String())
		}
	}
	set(map[string]any{"agree": true, "note": "first"})
	set(map[string]any{"agree": true, "note": ""})
	var mid struct {
		HumanNote string `json:"human_note"`
	}
	getReq := httptest.NewRequest(http.MethodGet, "/v0/settings/eval/cases/"+caseID, nil)
	getReq.Header.Set("Authorization", "Bearer adm")
	getRR := httptest.NewRecorder()
	h.ServeHTTP(getRR, getReq)
	_ = json.Unmarshal(getRR.Body.Bytes(), &mid)
	if mid.HumanNote != "" {
		t.Fatalf("note overwrite: %q", mid.HumanNote)
	}

	clearBody, _ := json.Marshal(map[string]any{"agree": nil})
	clearReq := httptest.NewRequest(http.MethodPatch, "/v0/settings/eval/cases/"+caseID+"/calibration", bytes.NewReader(clearBody))
	clearReq.Header.Set("Authorization", "Bearer adm")
	clearReq.Header.Set("Content-Type", "application/json")
	clearRR := httptest.NewRecorder()
	h.ServeHTTP(clearRR, clearReq)
	if clearRR.Code != http.StatusOK {
		t.Fatalf("clear: code=%d body=%s", clearRR.Code, clearRR.Body.String())
	}
	var cleared struct {
		HumanAgree *bool `json:"human_agree"`
	}
	_ = json.Unmarshal(clearRR.Body.Bytes(), &cleared)
	if cleared.HumanAgree != nil {
		t.Fatalf("after clear: %+v", cleared)
	}
}
