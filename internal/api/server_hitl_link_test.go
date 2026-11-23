package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/hitllink"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type hitlLinkRunner struct {
	store     store.Store
	decisions []run.Decision
}

func (f *hitlLinkRunner) Execute(context.Context, string, agent.Def, string) error { return nil }

func (f *hitlLinkRunner) ContinueFromHITL(_ context.Context, runID string, d run.Decision) error {
	f.decisions = append(f.decisions, d)
	status := store.StatusSucceeded
	if !d.Approve {
		status = store.StatusRejected
	}
	if err := f.store.SetHITL(runID, nil); err != nil {
		return err
	}
	return f.store.UpdateRun(runID, status, "", "")
}

type hitlLinkFixture struct {
	srv    *Server
	runner *hitlLinkRunner
	runID  string
	wait   time.Time
	token  string
}

func newHITLLinkFixture(t *testing.T, kind string) hitlLinkFixture {
	t.Helper()
	st := store.NewMemory()
	runner := &hitlLinkRunner{store: st}
	srv := NewServer(st, tool.NewRegistry(), runner)
	srv.CallbackSecret = []byte("link-secret")
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"

	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "create a ticket", ConversationID: "conv_link",
	})
	if err != nil {
		t.Fatal(err)
	}
	wait := time.Now().UTC().Add(-time.Minute)
	if err := st.SetHITL(r.ID, &store.HITLPayload{
		ToolName:     "create_ticket",
		Prompt:       "确认开单？",
		Kind:         kind,
		WaitingSince: wait,
	}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(r.ID, store.StatusWaitingHuman, "", ""); err != nil {
		t.Fatal(err)
	}
	token, _, err := hitllink.Issue(srv.CallbackSecret, r.ID, wait, 24*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return hitlLinkFixture{srv: srv, runner: runner, runID: r.ID, wait: wait, token: token}
}

func (f hitlLinkFixture) get(path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(rr, req)
	return rr
}

func (f hitlLinkFixture) post(path string, form url.Values) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	f.srv.Handler().ServeHTTP(rr, req)
	return rr
}

func TestHITLLinkGetPublicReadOnly(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindApproval)
	rr := f.get("/v0/hitl/approvals/" + f.token)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%q", ct)
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache-control=%q", rr.Header().Get("Cache-Control"))
	}
	if rr.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("referrer-policy=%q", rr.Header().Get("Referrer-Policy"))
	}
	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("nosniff=%q", rr.Header().Get("X-Content-Type-Options"))
	}
	if csp := rr.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "default-src 'none'") {
		t.Fatalf("csp=%q", csp)
	}
	body := rr.Body.String()
	for _, needle := range []string{"create_ticket", "确认开单？", f.runID} {
		if !strings.Contains(body, needle) {
			t.Fatalf("missing %q in body:\n%s", needle, body)
		}
	}
	got, _ := f.srv.Store.GetRun(f.runID)
	if got.Status != store.StatusWaitingHuman {
		t.Fatalf("GET mutated status=%s", got.Status)
	}
	if len(f.runner.decisions) != 0 {
		t.Fatalf("GET must not decide: %+v", f.runner.decisions)
	}
}

func TestHITLLinkPageDoesNotExposeArguments(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindApproval)
	_ = f.srv.Store.SetHITL(f.runID, &store.HITLPayload{
		ToolName:     "create_ticket",
		Prompt:       "确认开单？",
		Kind:         store.HITLKindApproval,
		WaitingSince: f.wait,
		Arguments:    map[string]any{"password": "super-secret-value"},
	})
	rr := f.get("/v0/hitl/approvals/" + f.token)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "super-secret-value") || strings.Contains(rr.Body.String(), "password") {
		t.Fatalf("tool arguments leaked into page:\n%s", rr.Body.String())
	}
}

func TestHITLLinkPostApprove(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindApproval)
	rr := f.post("/v0/hitl/approvals/"+f.token, url.Values{
		"decision": {"approve"},
		"comment":  {"looks good"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(f.runner.decisions) != 1 {
		t.Fatalf("decisions=%+v", f.runner.decisions)
	}
	d := f.runner.decisions[0]
	if !d.Approve || d.Comment != "looks good" || d.OperatorID != "hitl-link" {
		t.Fatalf("decision=%+v", d)
	}
	got, _ := f.srv.Store.GetRun(f.runID)
	if got.Status != store.StatusSucceeded {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestHITLLinkPostReject(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindApproval)
	rr := f.post("/v0/hitl/approvals/"+f.token, url.Values{
		"decision": {"reject"},
		"comment":  {"not now"},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(f.runner.decisions) != 1 || f.runner.decisions[0].Approve {
		t.Fatalf("decisions=%+v", f.runner.decisions)
	}
	if f.runner.decisions[0].Comment != "not now" {
		t.Fatalf("comment=%q", f.runner.decisions[0].Comment)
	}
	got, _ := f.srv.Store.GetRun(f.runID)
	if got.Status != store.StatusRejected {
		t.Fatalf("status=%s", got.Status)
	}
}

func TestHITLLinkReplayRejected(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindApproval)
	path := "/v0/hitl/approvals/" + f.token
	if rr := f.post(path, url.Values{"decision": {"approve"}}); rr.Code != http.StatusOK {
		t.Fatalf("first POST=%d body=%s", rr.Code, rr.Body.String())
	}
	rr := f.post(path, url.Values{"decision": {"approve"}})
	if rr.Code != http.StatusConflict {
		t.Fatalf("replay status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(f.runner.decisions) != 1 {
		t.Fatalf("replay decided again: %+v", f.runner.decisions)
	}
}

func TestHITLLinkRejectsLoginKind(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindLogin)
	rr := f.get("/v0/hitl/approvals/" + f.token)
	if rr.Code != http.StatusConflict {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "create_ticket") {
		t.Fatalf("must not render approval page for login: %s", rr.Body.String())
	}
	if len(f.runner.decisions) != 0 {
		t.Fatalf("login link decided: %+v", f.runner.decisions)
	}
}

func TestHITLLinkRejectsBadAndExpiredTokens(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindApproval)
	for _, token := range []string{"nope", f.token + "x"} {
		rr := f.get("/v0/hitl/approvals/" + token)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("token=%q status=%d body=%s", token, rr.Code, rr.Body.String())
		}
	}

	past := time.Now().UTC().Add(-2 * time.Hour)
	expired, _, err := hitllink.Issue([]byte("link-secret"), f.runID, past, time.Hour, past)
	if err != nil {
		t.Fatal(err)
	}
	rr := f.get("/v0/hitl/approvals/" + expired)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expired status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(f.runner.decisions) != 0 {
		t.Fatalf("bad token decided: %+v", f.runner.decisions)
	}
}

func TestHITLLinkRejectsWaitMismatch(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindApproval)
	stale, _, err := hitllink.Issue([]byte("link-secret"), f.runID, f.wait.Add(time.Second), 24*time.Hour, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rr := f.get("/v0/hitl/approvals/" + stale)
	if rr.Code != http.StatusConflict {
		t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
	}
	rr = f.post("/v0/hitl/approvals/"+stale, url.Values{"decision": {"approve"}})
	if rr.Code != http.StatusConflict {
		t.Fatalf("POST status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(f.runner.decisions) != 0 {
		t.Fatalf("stale wait decided: %+v", f.runner.decisions)
	}
}

func TestHITLLinkPostInvalidDecision(t *testing.T) {
	f := newHITLLinkFixture(t, store.HITLKindApproval)
	rr := f.post("/v0/hitl/approvals/"+f.token, url.Values{"decision": {"maybe"}})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if len(f.runner.decisions) != 0 {
		t.Fatalf("invalid decision decided: %+v", f.runner.decisions)
	}
}
