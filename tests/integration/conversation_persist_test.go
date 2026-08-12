package integration_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/agent"
	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// TestConversationMessagesAfterRun verifies that a single successful run
// persists both the user turn and the assistant terminal message, so
// GET /v0/conversations/{id}/messages returns at least 2 entries.
//
// Uses the bootstrap.StartForTest path (mock LLM + mock-ticket) so the
// full HTTP API → Engine → conversation store chain is exercised.
func TestConversationMessagesAfterRun(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "ticket-agent"
	cfg.Agent.System = "你是企业工单助手，只能通过工具访问工单系统。"
	cfg.Connector.ID = "ticket-api"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = filepath.Join("..", "..", "examples", "mock-ticket", "openapi.yaml")
	// list_tickets is a GET and not in require_approval, so a "查询" run
	// reaches succeeded without HITL.
	cfg.Run.MaxSteps = 8

	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	conv := "conv_persist_messages"
	body := `{"agent_id":"ticket-agent","input":"查询工单列表","conversation_id":"` + conv + `"}`
	resp, err := http.Post(runtimeURL+"/v0/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v0/runs: %v", err)
	}
	raw, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v0/runs status=%d body=%s", resp.StatusCode, raw)
	}
	var created struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil || created.RunID == "" {
		t.Fatalf("decode run_id: %v body=%s", err, raw)
	}

	pollRunStatus(t, runtimeURL, created.RunID, store.StatusSucceeded)

	// Give the terminal-message recorder a chance to land. The engine
	// appends the assistant message after UpdateRun(succeeded), so we
	// poll the messages endpoint until it has at least 2 entries.
	deadline := time.Now().Add(3 * time.Second)
	var msgs []json.RawMessage
	for time.Now().Before(deadline) {
		mr, err := http.Get(runtimeURL + "/v0/conversations/" + conv + "/messages")
		if err != nil {
			t.Fatalf("GET messages: %v", err)
		}
		mraw, _ := io.ReadAll(mr.Body)
		_ = mr.Body.Close()
		if mr.StatusCode != http.StatusOK {
			t.Fatalf("GET messages status=%d body=%s", mr.StatusCode, mraw)
		}
		if err := json.Unmarshal(mraw, &msgs); err != nil {
			t.Fatalf("decode messages: %v body=%s", err, mraw)
		}
		if len(msgs) >= 2 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(msgs) < 2 {
		t.Fatalf("expected >=2 messages after successful run, got %d: %s", len(msgs), mustJSON(t, msgs))
	}

	// Sanity: first persisted turn is the user input.
	var first struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(msgs[0], &first); err != nil {
		t.Fatal(err)
	}
	if first.Role != "user" || !strings.Contains(first.Content, "查询") {
		t.Fatalf("first message not user turn: %+v", first)
	}
}

// TestDeleteMessagesKeepsIdentities verifies that clearing conversation
// messages does not sign the user out: identities captured by login
// remain available. Exercises the HTTP API for DELETE messages and
// GET identities, with login capture driven through the tool registry
// (the mock LLM cannot drive a login tool, so we invoke it directly).
func TestDeleteMessagesKeepsIdentities(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/login":
			_, _ = w.Write([]byte(`{"accessToken":"conv-persist-token","email":"admin@x.com"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/me":
			_, _ = w.Write([]byte(`{"ok":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(upstream.Close)

	specPath := writeConversationPersistSpec(t)
	dbPath := filepath.Join(t.TempDir(), "baize.db")
	sqlite, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = sqlite.Close() })

	reg := tool.NewRegistry()
	idStore, err := identity.OpenSQLite(sqlite.DB())
	if err != nil {
		t.Fatalf("open identity sqlite: %v", err)
	}
	if _, _, err := openapi.RegisterWithOpts(sqlite, reg, openapi.RegisterOpts{
		ID:         "conv-persist-api",
		Type:       "openapi",
		SpecPath:   specPath,
		BaseURL:    upstream.URL,
		Identities: idStore,
		Resolver:   authresolve.OpenAPISecurityResolver{},
		Capture: identity.CaptureConfig{
			ToolNameGlob:   "*login*",
			TokenJSONPaths: []string{"accessToken", "data.token"},
			LabelJSONPaths: []string{"email"},
			HeaderTemplate: "Bearer {{token}}",
			DefaultScheme:  "bearer",
		},
	}); err != nil {
		t.Fatalf("register connector: %v", err)
	}

	convStore, err := conversation.OpenSQLite(sqlite.DB())
	if err != nil {
		t.Fatalf("open conversation sqlite: %v", err)
	}
	conv := "conv_persist_delete"
	if _, err := convStore.Append(conv, conversation.Message{Role: conversation.RoleUser, Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	apiSrv := api.NewServer(sqlite, reg, &fakeRunner{})
	apiSrv.Identities = idStore
	apiSrv.Messages = convStore
	h := apiSrv.Handler()

	// 1) Drive login capture through the registry (mock LLM cannot).
	ctx := identity.WithConversationID(context.Background(), conv)
	if _, isErr, err := reg.Invoke(ctx, "login", map[string]any{"email": "admin@x.com", "password": "x"}); err != nil || isErr {
		t.Fatalf("login: isErr=%v err=%v", isErr, err)
	}
	views := idStore.ListPublic(conv)
	if len(views) != 1 {
		t.Fatalf("expected 1 captured identity, got %+v", views)
	}
	capturedID := views[0].ID

	// 2) GET identities confirms the captured account is visible.
	idResp := getOKBody(t, h, "/v0/conversations/"+conv+"/identities")
	var public []identity.PublicView
	if err := json.NewDecoder(strings.NewReader(idResp)).Decode(&public); err != nil {
		t.Fatalf("decode identities: %v body=%s", err, idResp)
	}
	if len(public) != 1 || public[0].ID != capturedID {
		t.Fatalf("public identities=%+v", public)
	}

	// 3) DELETE messages must not touch identities.
	del := httptest.NewRequest(http.MethodDelete, "/v0/conversations/"+conv+"/messages", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, del)
	if rr.Code != http.StatusOK {
		t.Fatalf("DELETE messages status=%d body=%s", rr.Code, rr.Body.String())
	}

	// 4) Identities still present after clearing messages.
	if got := idStore.ListPublic(conv); len(got) != 1 || got[0].ID != capturedID {
		t.Fatalf("identities changed after DELETE messages: %+v", got)
	}
	idResp2 := getOKBody(t, h, "/v0/conversations/"+conv+"/identities")
	if !strings.Contains(idResp2, capturedID) {
		t.Fatalf("identities body after delete missing id: %s", idResp2)
	}

	// 5) Messages are actually gone.
	msgResp := getOKBody(t, h, "/v0/conversations/"+conv+"/messages")
	if !strings.HasPrefix(strings.TrimSpace(msgResp), "[]") {
		t.Fatalf("expected empty messages list, got %s", msgResp)
	}
}

// TestIdentitySQLiteReopen verifies that an identity written to a SQLite
// file survives Close → Open, surfaced via ListPublic.
func TestIdentitySQLiteReopen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "identities.db")

	db1, err := openSQLiteDB(t, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	s1, err := identity.OpenSQLite(db1)
	if err != nil {
		t.Fatal(err)
	}
	id, err := s1.Upsert("conv_reopen", identity.Identity{
		Label:             "admin@x.com",
		Scheme:            "bearer",
		Subject:           "admin@x.com",
		Source:            identity.SourceLoginCapture,
		CredentialHeaders: map[string]string{"Authorization": "Bearer token-xyz"},
		IsDefault:         true,
	})
	if err != nil || id == "" {
		t.Fatalf("upsert: id=%q err=%v", id, err)
	}
	if err := db1.Close(); err != nil {
		t.Fatal(err)
	}

	db2, err := openSQLiteDB(t, dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db2.Close()
	s2, err := identity.OpenSQLite(db2)
	if err != nil {
		t.Fatal(err)
	}
	views := s2.ListPublic("conv_reopen")
	if len(views) != 1 || views[0].ID != id || views[0].Label != "admin@x.com" || !views[0].IsDefault {
		t.Fatalf("ListPublic after reopen = %+v, want 1 entry id=%s", views, id)
	}
}

// fakeRunner satisfies api.Runner for tests that drive tools directly
// through the registry instead of the engine.
type fakeRunner struct{}

func (fakeRunner) Execute(context.Context, string, agent.Def, string) error {
	return nil
}

func (fakeRunner) ContinueFromHITL(context.Context, string, run.Decision) error {
	return nil
}

func writeConversationPersistSpec(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "conv-persist.yaml")
	content := `openapi: 3.0.3
info:
  title: conv-persist
  version: 0.1.0
components:
  securitySchemes:
    bearer:
      type: http
      scheme: bearer
paths:
  /login:
    post:
      operationId: login
      security: []
      requestBody:
        content:
          application/json:
            schema:
              type: object
              properties:
                email: { type: string }
                password: { type: string }
      responses:
        "200":
          description: ok
  /me:
    get:
      operationId: getMe
      security:
        - bearer: []
      responses:
        "200":
          description: ok`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func openSQLiteDB(t *testing.T, path string) (*sql.DB, error) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(`PRAGMA busy_timeout=5000`); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func pollRunStatus(t *testing.T, runtimeURL, runID string, want store.Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		resp, err := http.Get(runtimeURL + "/v0/runs/" + runID)
		if err != nil {
			t.Fatalf("GET run: %v", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var r store.Run
		if err := json.Unmarshal(raw, &r); err != nil {
			t.Fatalf("decode run: %v body=%s", err, raw)
		}
		last = string(r.Status)
		if r.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run status=%q want %s", last, want)
}

func getOKBody(t *testing.T, h http.Handler, path string) string {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET %s status=%d body=%s", path, rr.Code, rr.Body.String())
	}
	return rr.Body.String()
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
