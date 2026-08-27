package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// waitingFakeRunner transitions the run to StatusWaitingHuman on Execute and
// to StatusSucceeded on ContinueFromHITL, so the resume path can be exercised
// inside package api (where the api_test capturingRunner is unreachable).
type waitingFakeRunner struct {
	store store.Store
}

func (f *waitingFakeRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	return f.store.UpdateRun(runID, store.StatusWaitingHuman, "", "")
}

func (f *waitingFakeRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	return f.store.UpdateRun(runID, store.StatusSucceeded, "resumed", "")
}

// TestGateOnPassthroughPostRunDoesNotStoreControlPlaneToken: 门禁开着且
// passthrough 时，控制面口令只用于过门禁，不得被 PickHeaders 收进 Run 的
// passthrough_json（否则无 conversation_id 的机器路径会把它打到下游 API）。
func TestGateOnPassthroughPostRunDoesNotStoreControlPlaneToken(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "s"})
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.OperatorToken = "op-secret"
	srv.AdminToken = "adm-secret"
	srv.AuthMode = "passthrough"
	h := srv.Handler()

	body := strings.NewReader(`{"agent_id":"a","input":"hi"}`)
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", body)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer op-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	if runID == "" {
		t.Fatalf("created=%v", created)
	}

	got, _ := srv.Store.GetRun(runID)
	if got == nil {
		t.Fatalf("run %s not found", runID)
	}
	if v, ok := got.PassthroughHeaders["Authorization"]; ok {
		t.Fatalf("control-plane Authorization leaked into store: %q headers=%+v", v, got.PassthroughHeaders)
	}
	if strings.Contains(fmt.Sprintf("%+v", got.PassthroughHeaders), "op-secret") {
		t.Fatalf("control-plane token value leaked into store: %+v", got.PassthroughHeaders)
	}
}

// TestGateOnPassthroughResumeDoesNotStoreControlPlaneToken: 门禁开着且
// passthrough 时，resume 请求上的控制面口令同样不得写入 Store。
func TestGateOnPassthroughResumeDoesNotStoreControlPlaneToken(t *testing.T) {
	st := store.NewMemory()
	st.UpsertAgent(store.Agent{ID: "a", System: "s"})
	srv := NewServer(st, tool.NewRegistry(), &waitingFakeRunner{store: st})
	srv.OperatorToken = "op-secret"
	srv.AuthMode = "passthrough"
	h := srv.Handler()

	postReq := httptest.NewRequest(http.MethodPost, "/v0/runs",
		strings.NewReader(`{"agent_id":"a","input":"hi"}`))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Authorization", "Bearer op-secret")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, postReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("post status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&created)
	runID, _ := created["run_id"].(string)
	if runID == "" {
		t.Fatalf("created=%v", created)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		r, err := st.GetRun(runID)
		if err == nil && r.Status == store.StatusWaitingHuman {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	resumeReq := httptest.NewRequest(http.MethodPost, "/v0/runs/"+runID+"/resume",
		strings.NewReader(`{"decision":"approve","comment":"ok"}`))
	resumeReq.Header.Set("Content-Type", "application/json")
	resumeReq.Header.Set("Authorization", "Bearer op-secret")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, resumeReq)
	if rr.Code != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", rr.Code, rr.Body.String())
	}

	got, _ := st.GetRun(runID)
	if got == nil {
		t.Fatalf("run %s not found", runID)
	}
	if v, ok := got.PassthroughHeaders["Authorization"]; ok {
		t.Fatalf("control-plane Authorization leaked into store via resume: %q headers=%+v", v, got.PassthroughHeaders)
	}
	if strings.Contains(fmt.Sprintf("%+v", got.PassthroughHeaders), "op-secret") {
		t.Fatalf("control-plane token value leaked into store via resume: %+v", got.PassthroughHeaders)
	}
}
