package httpplugin_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestHTTPPluginConversationSkipsStaticAndRespectsLogin(t *testing.T) {
	var lastAuth string
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo"},{"name":"secret_op"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			hits++
			lastAuth = r.Header.Get("Authorization")
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	if _, _, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{
		ID: "side", BaseURL: srv.URL,
		Headers:      map[string]string{"Authorization": "Bearer PLUGIN_ENV"},
		RequireLogin: []string{"secret_op"},
		Identities:   ids,
		Resolver:     authresolve.OpenAPISecurityResolver{},
	}); err != nil {
		t.Fatal(err)
	}
	ctx := identity.WithConversationID(context.Background(), "c1")
	hits, lastAuth = 0, ""
	_, isErr, err := reg.Invoke(ctx, "echo", nil)
	if err != nil || isErr {
		t.Fatal(err)
	}
	if lastAuth == "Bearer PLUGIN_ENV" {
		t.Fatal("conversation must not send plugin static token")
	}
	content, isErr, err := reg.Invoke(ctx, "secret_op", nil)
	if err != nil || !isErr || content["code"] != "login_required" {
		t.Fatalf("%v %v %v", content, isErr, err)
	}
	if _, err := ids.Upsert("c1", identity.Identity{
		Scheme: "bearer", Subject: "u",
		CredentialHeaders: map[string]string{"Authorization": "Bearer CAP"},
		Source:            identity.SourceLoginCapture,
		IsDefault:         true,
	}); err != nil {
		t.Fatal(err)
	}
	hits, lastAuth = 0, ""
	_, isErr, err = reg.Invoke(ctx, "secret_op", nil)
	if err != nil || isErr {
		t.Fatal(err)
	}
	if lastAuth != "Bearer CAP" {
		t.Fatalf("auth=%q", lastAuth)
	}
}

func TestRegisterListsToolsAndInvoke(t *testing.T) {
	var lastAuth, lastRun, lastPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		lastRun = r.Header.Get("X-Baize-Run-Id")
		lastPath = r.URL.Path
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	_, infos, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{
		ID: "side", BaseURL: srv.URL, Headers: map[string]string{"Authorization": "Bearer T"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 1 || infos[0].Name != "echo" || infos[0].ConnectorID != "side" {
		t.Fatalf("%+v", infos)
	}
	if infos[0].Method != "" {
		t.Fatalf("method should be empty: %+v", infos[0])
	}
	ctx := identity.WithRunID(context.Background(), "run_9")
	content, isErr, invErr := reg.Invoke(ctx, "echo", map[string]any{"a": 1})
	if invErr != nil || isErr || content["ok"] != true {
		t.Fatalf("invoke %v %v %v", content, isErr, invErr)
	}
	if lastAuth != "Bearer T" || lastRun != "run_9" || !strings.Contains(lastPath, "echo") {
		t.Fatalf("auth=%s run=%s path=%s", lastAuth, lastRun, lastPath)
	}
	c, err := st.GetConnector("side")
	if err != nil || c.Type != "http" || c.Spec != "" {
		t.Fatalf("store %+v err=%v", c, err)
	}
}

func TestRegisterHealthzFailDoesNotRegister(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", 500)
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.RegisterMeta(tool.Meta{Spec: llm.ToolSpec{Name: "keep"}, ConnectorID: "other"},
		func(context.Context, map[string]any) (map[string]any, bool, error) { return nil, false, nil }, false)
	_, _, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{ID: "side", BaseURL: srv.URL})
	if !errors.Is(err, httpplugin.ErrInvalidPlugin) {
		t.Fatalf("err=%v", err)
	}
	if len(reg.List()) != 1 || reg.List()[0].Name != "keep" {
		t.Fatalf("polluted %+v", reg.List())
	}
}

func TestRegisterConflict(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	reg.RegisterMeta(tool.Meta{Spec: llm.ToolSpec{Name: "echo"}, ConnectorID: "other"},
		func(context.Context, map[string]any) (map[string]any, bool, error) { return nil, false, nil }, false)
	_, _, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{ID: "side", BaseURL: srv.URL})
	if !errors.Is(err, httpplugin.ErrToolConflict) {
		t.Fatalf("err=%v", err)
	}
	if len(reg.List()) != 1 || reg.List()[0].Name != "echo" || reg.List()[0].ConnectorID != "other" {
		t.Fatalf("other should remain: %+v", reg.List())
	}
}

func TestRegisterRequireApproval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[
				{"name":"create_ticket","description":"create"},
				{"name":"echo","description":"echo","annotations":{"dangerous":true}}
			]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	_, _, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{
		ID: "side", BaseURL: srv.URL, RequireApproval: []string{"create_ticket"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reg.RequiresApproval("create_ticket") {
		t.Fatal("create_ticket must require approval via RequireApproval list")
	}
	if reg.RequiresApproval("echo") {
		t.Fatal("echo with annotations.dangerous must not require approval unless listed")
	}
}

const registerHTTPCaptureJWT = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJhZG1pbkB4LmNvbSIsImVtYWlsIjoiYWRtaW5AeC5jb20iLCJleHAiOjk5OTk5OTk5OTl9.sig"

func TestRegisterWithOptsCapturesLogin(t *testing.T) {
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"login","description":"login"},{"name":"ping","description":"ping"}]}`))
		case strings.HasSuffix(r.URL.Path, "/login/invoke"):
			_, _ = w.Write([]byte(`{"content":{"accessToken":"` + registerHTTPCaptureJWT + `","email":"admin@x.com"},"is_error":false}`))
		case strings.HasSuffix(r.URL.Path, "/ping/invoke"):
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	_, _, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{
		ID:           "side",
		BaseURL:      srv.URL,
		Headers:      map[string]string{"Authorization": "Bearer PLUGIN_ENV"},
		RequireLogin: []string{"ping"},
		Identities:   ids,
		Resolver:     authresolve.OpenAPISecurityResolver{},
		Capture: identity.CaptureConfig{
			ToolNameGlob:   "*login*",
			TokenJSONPaths: []string{"accessToken"},
			LabelJSONPaths: []string{"email"},
			HeaderTemplate: "Bearer {{token}}",
			DefaultScheme:  "bearer",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := identity.WithConversationID(context.Background(), "conv_http_reg")
	_, isErr, err := reg.Invoke(ctx, "login", map[string]any{})
	if err != nil || isErr {
		t.Fatalf("login: isErr=%v err=%v", isErr, err)
	}
	if len(ids.List("conv_http_reg")) == 0 {
		t.Fatal("expected captured identity after login")
	}

	lastAuth = ""
	_, isErr, err = reg.Invoke(ctx, "ping", nil)
	if err != nil || isErr {
		t.Fatalf("ping: isErr=%v err=%v", isErr, err)
	}
	wantCaptured := "Bearer " + registerHTTPCaptureJWT
	if lastAuth != wantCaptured {
		t.Fatalf("after-login Authorization=%q, want %q", lastAuth, wantCaptured)
	}
}

func TestRegisterPassthroughUsesContextHeaders(t *testing.T) {
	var lastAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastAuth = r.Header.Get("Authorization")
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	_, _, err := httpplugin.RegisterWithOpts(st, reg, httpplugin.RegisterOpts{
		ID: "side", BaseURL: srv.URL, AuthMode: "passthrough",
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := identity.WithPassthroughHeaders(context.Background(), map[string]string{
		"Authorization": "Bearer FROM_RUN",
	})
	content, isErr, invErr := reg.Invoke(ctx, "echo", map[string]any{})
	if invErr != nil || isErr || content["ok"] != true {
		t.Fatalf("invoke content=%v isErr=%v err=%v", content, isErr, invErr)
	}
	if lastAuth != "Bearer FROM_RUN" {
		t.Fatalf("Authorization=%q want Bearer FROM_RUN", lastAuth)
	}
}
