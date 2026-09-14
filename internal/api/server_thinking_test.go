package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestPostRunRejectsBadThinkingLevel(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	req := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{
			"agent_id":       "a",
			"input":          "x",
			"thinking_level": "ultra",
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "invalid_request" {
		t.Fatalf("code=%q want invalid_request", wrap.Error.Code)
	}
}

func TestPostRunStoresThinkingLevel(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := api.NewServer(st, reg, &fakeRunner{store: st})
	h := srv.Handler()

	st.UpsertAgent(store.Agent{ID: "a", System: "s"})

	reqHigh := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{
			"agent_id":       "a",
			"input":          "x",
			"thinking_level": "high",
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, reqHigh)
	if rr.Code != http.StatusOK {
		t.Fatalf("high status=%d body=%s", rr.Code, rr.Body.String())
	}
	var highResp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&highResp); err != nil {
		t.Fatal(err)
	}
	runID, _ := highResp["run_id"].(string)
	got, err := st.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ThinkingLevel != "high" {
		t.Fatalf("ThinkingLevel=%q want high", got.ThinkingLevel)
	}

	reqOmit := httptest.NewRequest(http.MethodPost, "/v0/runs",
		jsonBody(t, map[string]any{"agent_id": "a", "input": "y"}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, reqOmit)
	if rr.Code != http.StatusOK {
		t.Fatalf("omit status=%d body=%s", rr.Code, rr.Body.String())
	}
	var omitResp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&omitResp); err != nil {
		t.Fatal(err)
	}
	runID2, _ := omitResp["run_id"].(string)
	got2, err := st.GetRun(runID2)
	if err != nil {
		t.Fatal(err)
	}
	if got2.ThinkingLevel != "" {
		t.Fatalf("omitted ThinkingLevel=%q want empty", got2.ThinkingLevel)
	}
}
