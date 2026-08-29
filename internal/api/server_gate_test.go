package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// gateFakeRunner is the package-api counterpart of fakeRunner in
// server_test.go (package api_test). Kept local so the gate tests can reach
// Server's unexported token fields without crossing package boundaries.
type gateFakeRunner struct {
	store store.Store
}

func (f *gateFakeRunner) Execute(ctx context.Context, runID string, ag agent.Def, input string) error {
	_ = f.store.AppendEvent(runID, store.Event{Type: "run.started"})
	return f.store.UpdateRun(runID, store.StatusSucceeded, "已创建", "")
}

func (f *gateFakeRunner) ContinueFromHITL(ctx context.Context, runID string, d run.Decision) error {
	return nil
}

func gateServer(t *testing.T, operator, admin string) *Server {
	t.Helper()
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.OperatorToken = operator
	srv.AdminToken = admin
	return srv
}

func TestGateOffAllowsAnonymous(t *testing.T) {
	srv := gateServer(t, "", "")
	req := httptest.NewRequest(http.MethodGet, "/v0/ui-config", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code=%d", rr.Code)
	}
	var body struct {
		GateEnabled bool `json:"gate_enabled"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.GateEnabled {
		t.Fatal("gate must be off")
	}
	req = httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(`{"agent_id":"a","input":"i"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	// 现有无门禁 handlePostRun 对未知 agent_id 返回 404（agent_not_found）。
	// 门关着时行为必须与今天相同：只要不是 401/403 即可，不改变 Run 创建语义。
	if rr.Code == 401 || rr.Code == 403 {
		t.Fatalf("anonymous POST /v0/runs must not be gated: code=%d body=%s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/me", nil)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"role":""`) || !strings.Contains(rr.Body.String(), `"gate_enabled":false`) {
		t.Fatalf("me=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateOnUnauthorized(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("healthz=%d", rr.Code)
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/ui-config", nil)
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"gate_enabled":true`) {
		t.Fatalf("ui-config=%d %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(`{"agent_id":"a","input":"i"}`))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 || !strings.Contains(rr.Body.String(), "unauthorized") {
		t.Fatalf("want 401 unauthorized, got %d %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Header().Get("WWW-Authenticate"), "Basic") {
		t.Fatal("must not send WWW-Authenticate Basic")
	}
	req = httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(`{"agent_id":"a","input":"i"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer nope")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 || !strings.Contains(rr.Body.String(), "需要控制面口令") {
		t.Fatalf("bad token: %d %s", rr.Code, rr.Body.String())
	}
}

func TestGateOperatorForbiddenOnAdminRoutes(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()
	auth := func(r *http.Request) { r.Header.Set("Authorization", "Bearer op") }

	req := httptest.NewRequest(http.MethodGet, "/v0/me", nil)
	auth(req)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"role":"operator"`) || !strings.Contains(rr.Body.String(), `"operator_id":"operator"`) {
		t.Fatalf("me=%d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPost, "/v0/runs", strings.NewReader(`{"agent_id":"a","input":"i"}`))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	// 现有 handlePostRun 对未知 agent_id 返回 404；operator 通过门禁即可，
	// 不改变 Run 创建语义，只要不是 401/403。
	if rr.Code == 401 || rr.Code == 403 {
		t.Fatalf("POST runs must not be blocked for operator: code=%d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	auth(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("GET tools=%d %s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodPut, "/v0/connectors/x", strings.NewReader(`{"type":"openapi","spec":"examples/mock-ticket/openapi.yaml","base_url":"http://127.0.0.1:9"}`))
	req.Header.Set("Content-Type", "application/json")
	auth(req)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "需要管理员口令") {
		t.Fatalf("PUT connector=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateAdminCanReadTools(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	req := httptest.NewRequest(http.MethodGet, "/v0/me", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"role":"admin"`) {
		t.Fatalf("me=%d %s", rr.Code, rr.Body.String())
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("tools=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateOperatorOnlyCannotPut(t *testing.T) {
	srv := gateServer(t, "op", "")
	req := httptest.NewRequest(http.MethodPut, "/v0/agents/a1", strings.NewReader(`{"system":"x"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 403 {
		t.Fatalf("code=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateSameTokenIsAdmin(t *testing.T) {
	srv := gateServer(t, "same", "same")
	req := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	req.Header.Set("Authorization", "Bearer same")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateNamedOperatorMe(t *testing.T) {
	srv := gateServer(t, "", "adm")
	srv.Operators = []controlplane.Operator{{ID: "alice", Token: "ta"}, {ID: "bob", Token: "tb"}}
	req := httptest.NewRequest(http.MethodGet, "/v0/me", nil)
	req.Header.Set("Authorization", "Bearer ta")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), `"operator_id":"alice"`) {
		t.Fatalf("me=%d %s", rr.Code, rr.Body.String())
	}
}

func TestGateSSEUnauthorizedNotEventStream(t *testing.T) {
	st := store.NewMemory()
	run, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.OperatorToken = "op"
	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 401 {
		t.Fatalf("code=%d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		t.Fatalf("ct=%s", ct)
	}
}

// TestGateOperatorForbiddenOnCatalogWrites: with the gate on, an operator
// token must be rejected (403) on POST and DELETE catalog endpoints, while an
// admin token is allowed through the gate (handler-level 4xx is fine).
func TestGateOperatorForbiddenOnCatalogWrites(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()

	post := httptest.NewRequest(http.MethodPost, "/v0/connectors/c1/tools",
		strings.NewReader(`{"name":"x","method":"GET","path":"/x"}`))
	post.Header.Set("Content-Type", "application/json")
	post.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, post)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator POST tools: %d %s", rr.Code, rr.Body.String())
	}

	del := httptest.NewRequest(http.MethodDelete, "/v0/connectors/c1/tools/x", nil)
	del.Header.Set("Authorization", "Bearer op")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, del)
	if rr.Code != 403 || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator DELETE tools: %d %s", rr.Code, rr.Body.String())
	}

	// Admin passes the gate; handler then returns 404 for the unknown connector.
	postAdmin := httptest.NewRequest(http.MethodPost, "/v0/connectors/c1/tools",
		strings.NewReader(`{"name":"x","method":"GET","path":"/x"}`))
	postAdmin.Header.Set("Content-Type", "application/json")
	postAdmin.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, postAdmin)
	if rr.Code == 401 || rr.Code == 403 {
		t.Fatalf("admin must pass the gate: %d %s", rr.Code, rr.Body.String())
	}
}

func TestGateSSEOperatorOK(t *testing.T) {
	st := store.NewMemory()
	run, _ := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	_ = st.UpdateRun(run.ID, store.StatusSucceeded, "done", "")
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.OperatorToken = "op"
	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("code=%d %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/event-stream") {
		t.Fatalf("ct=%s", rr.Header().Get("Content-Type"))
	}
}
