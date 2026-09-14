package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestLoginEntriesAndInvoke(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_, _ = w.Write([]byte(`{"accessToken":"` + putCaptureJWT + `","email":"admin@x.com"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/me":
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	spec := writeLoginGetMeAPISpec(t)
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	msgStore := conversation.NewMemoryStore()
	eng := &run.Engine{
		Store:      st,
		LLM:        &fatalChatLLM{t: t},
		Tools:      reg,
		Gate:       run.NewGate(),
		Identities: ids,
	}
	srv := api.NewServer(st, reg, eng)
	srv.Identities = ids
	srv.Messages = msgStore
	srv.DefaultAgentID = "default-agent"
	st.UpsertAgent(store.Agent{ID: "default-agent", System: "sys"})
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/connectors/auth",
		jsonBody(t, map[string]any{
			"type":     "openapi",
			"spec":     spec,
			"base_url": upstream.URL,
			"auth": map[string]any{
				"mode": "static",
				"static": map[string]any{
					"headers": map[string]string{"Authorization": "Bearer PUT_STATIC"},
				},
			},
		}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("PUT connector status=%d body=%s", rr.Code, rr.Body.String())
	}

	// GET login-entries → contains login entry
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/login-entries", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET entries status=%d body=%s", rr.Code, rr.Body.String())
	}
	var listBody struct {
		Entries []map[string]any `json:"entries"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Entries) != 1 {
		t.Fatalf("entries=%d want 1; body=%v", len(listBody.Entries), listBody.Entries)
	}
	entry := listBody.Entries[0]
	if entry["tool_name"] != "login" || entry["connector_id"] != "auth" {
		t.Fatalf("entry=%v", entry)
	}

	// Ensure login tool has required args so ValidateArgs can be exercised.
	loginTool, err := st.GetTool("login")
	if err != nil {
		t.Fatal(err)
	}
	schema := loginTool.InputSchema
	if schema == nil {
		schema = map[string]any{"type": "object"}
	}
	schema["required"] = []any{"email", "password"}
	loginTool.InputSchema = schema
	st.UpsertTool(loginTool)

	// GET ?connector_id=other → entries:[]
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/login-entries?connector_id=other", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET filtered status=%d body=%s", rr.Code, rr.Body.String())
	}
	listBody = struct {
		Entries []map[string]any `json:"entries"`
	}{}
	if err := json.NewDecoder(rr.Body).Decode(&listBody); err != nil {
		t.Fatal(err)
	}
	if len(listBody.Entries) != 0 {
		t.Fatalf("filtered entries=%v want []", listBody.Entries)
	}

	// POST login-invoke missing agent_id → 400
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/conversations/c1/login-invoke",
		jsonBody(t, map[string]any{
			"connector_id": "auth",
			"tool_name":    "login",
			"arguments":    map[string]any{"email": "a@x.com", "password": "x"},
		})))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("missing agent_id status=%d body=%s", rr.Code, rr.Body.String())
	}

	// POST login-invoke missing required args → 400
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/conversations/c1/login-invoke",
		jsonBody(t, map[string]any{
			"agent_id":     "default-agent",
			"connector_id": "auth",
			"tool_name":    "login",
			"arguments":    map[string]any{"email": "a@x.com"},
		})))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("ValidateArgs status=%d body=%s", rr.Code, rr.Body.String())
	}

	// POST valid → 200 {run_id}; poll events have tool.result; GET identities have login_capture
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/conversations/c1/login-invoke",
		jsonBody(t, map[string]any{
			"agent_id":     "default-agent",
			"connector_id": "auth",
			"tool_name":    "login",
			"arguments":    map[string]any{"email": "admin@x.com", "password": "secret"},
		})))
	if rr.Code != http.StatusOK {
		t.Fatalf("invoke status=%d body=%s", rr.Code, rr.Body.String())
	}
	var created map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	runID, _ := created["run_id"].(string)
	if runID == "" {
		t.Fatalf("missing run_id: %v", created)
	}
	if created["conversation_id"] != "c1" {
		t.Fatalf("conversation_id=%v", created["conversation_id"])
	}
	waitForStatus(t, st, runID, store.StatusSucceeded)

	msgs := msgStore.List("c1")
	if len(msgs) == 0 || !strings.Contains(msgs[0].Content, "已发起登录") {
		t.Fatalf("bubble message=%v", msgs)
	}
	if strings.Contains(msgs[0].Content, "secret") || strings.Contains(msgs[0].Content, "password") {
		t.Fatalf("bubble must not contain arguments: %q", msgs[0].Content)
	}

	evs, err := st.ListEvents(runID)
	if err != nil {
		t.Fatal(err)
	}
	var sawResult bool
	for _, ev := range evs {
		if ev.Type == run.EventToolResult {
			sawResult = true
			break
		}
	}
	if !sawResult {
		t.Fatalf("expected tool.result in events: %+v", evs)
	}

	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/v0/conversations/c1/identities", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("GET identities status=%d body=%s", rr.Code, rr.Body.String())
	}
	var views []identity.PublicView
	if err := json.NewDecoder(rr.Body).Decode(&views); err != nil {
		t.Fatal(err)
	}
	if len(views) == 0 || views[0].Source != identity.SourceLoginCapture {
		t.Fatalf("identities=%+v want login_capture", views)
	}

	// Concurrent: CreateRun occupies conversation, then invoke → 409 conversation_busy
	blocker, err := st.CreateRun(store.CreateRunInput{AgentID: "default-agent", ConversationID: "c1"})
	if err != nil {
		t.Fatal(err)
	}
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/conversations/c1/login-invoke",
		jsonBody(t, map[string]any{
			"agent_id":     "default-agent",
			"connector_id": "auth",
			"tool_name":    "login",
			"arguments":    map[string]any{"email": "a@x.com", "password": "x"},
		})))
	if rr.Code != http.StatusConflict {
		t.Fatalf("busy status=%d body=%s", rr.Code, rr.Body.String())
	}
	var wrap struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "conversation_busy" {
		t.Fatalf("code=%q want conversation_busy", wrap.Error.Code)
	}

	// Finish the occupying run so not_a_login_entry can be tested cleanly.
	_ = st.UpdateRun(blocker.ID, store.StatusSucceeded, "", "")

	// POST non-catalog tool → 400 not_a_login_entry
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/v0/conversations/c1/login-invoke",
		jsonBody(t, map[string]any{
			"agent_id":     "default-agent",
			"connector_id": "auth",
			"tool_name":    "getMe",
			"arguments":    map[string]any{},
		})))
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("not_a_login_entry status=%d body=%s", rr.Code, rr.Body.String())
	}
	wrap = struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}{}
	if err := json.NewDecoder(rr.Body).Decode(&wrap); err != nil {
		t.Fatal(err)
	}
	if wrap.Error.Code != "not_a_login_entry" {
		t.Fatalf("code=%q want not_a_login_entry", wrap.Error.Code)
	}
}

// fatalChatLLM fails if the model is consulted during forced login invoke.
type fatalChatLLM struct{ t *testing.T }

func (f *fatalChatLLM) Chat(context.Context, []llm.Message, []llm.ToolSpec) (llm.Message, error) {
	f.t.Fatal("LLM Chat must not be called for login-invoke")
	return llm.Message{}, nil
}

func (f *fatalChatLLM) SupportsVision() bool { return false }
