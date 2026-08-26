package httpplugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/connector/httpplugin"
)

func TestClientHealthzAndListTools(t *testing.T) {
	var sawProto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawProto = r.Header.Get("X-Baize-Protocol")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/healthz":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v0/tools":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"d"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := httpplugin.NewClient(srv.URL)
	if err := c.Healthz(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sawProto != "v0" {
		t.Fatalf("protocol=%q", sawProto)
	}
	tools, err := c.ListTools(context.Background())
	if err != nil || len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools=%+v err=%v", tools, err)
	}
}

func TestClientHealthzNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"down"}`))
	}))
	defer srv.Close()
	err := httpplugin.NewClient(srv.URL).Healthz(context.Background())
	if !errors.Is(err, httpplugin.ErrInvalidPlugin) {
		t.Fatalf("err=%v", err)
	}
}

func TestClientInvokeSendsContextAndHeaders(t *testing.T) {
	var gotBody map[string]any
	var gotRunHdr, gotAuth, gotProto string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProto = r.Header.Get("X-Baize-Protocol")
		gotRunHdr = r.Header.Get("X-Baize-Run-Id")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
	}))
	defer srv.Close()
	c := httpplugin.NewClient(srv.URL)
	out, err := c.Invoke(context.Background(), "echo", map[string]any{"x": 1}, httpplugin.InvokeMeta{
		RunID:   "run_1",
		AgentID: "ag_1",
		Headers: map[string]string{"Authorization": "Bearer TOK"},
	})
	if err != nil || out.IsError || out.Content["ok"] != true {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if gotProto != "v0" || gotRunHdr != "run_1" || gotAuth != "Bearer TOK" {
		t.Fatalf("hdr proto=%s run=%s auth=%s", gotProto, gotRunHdr, gotAuth)
	}
	ctx, _ := gotBody["context"].(map[string]any)
	if ctx["run_id"] != "run_1" || ctx["agent_id"] != "ag_1" {
		t.Fatalf("body=%+v", gotBody)
	}
	if _, ok := ctx["callback_urls"]; ok {
		t.Fatalf("callback_urls must be absent when CallbackEventURL empty: %+v", ctx)
	}
}

func TestClientInvokeSendsCallbackEventURL(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
	}))
	defer srv.Close()
	c := httpplugin.NewClient(srv.URL)
	const want = "https://base.example/v0/runs/run_2/plugin-callbacks?token=abc.def"
	if _, err := c.Invoke(context.Background(), "echo", nil, httpplugin.InvokeMeta{
		RunID:            "run_2",
		CallbackEventURL: want,
	}); err != nil {
		t.Fatal(err)
	}
	ctx, _ := gotBody["context"].(map[string]any)
	urls, _ := ctx["callback_urls"].(map[string]any)
	if urls == nil || urls["event"] != want {
		t.Fatalf("callback_urls=%+v ctx=%+v", urls, ctx)
	}
	if ctx["run_id"] != "run_2" {
		t.Fatalf("run_id missing: %+v", ctx)
	}
}

func TestClientInvokeHTTPErrorIsToolError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`oops`))
	}))
	defer srv.Close()
	out, err := httpplugin.NewClient(srv.URL).Invoke(context.Background(), "echo", nil, httpplugin.InvokeMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Fatalf("want is_error out=%+v", out)
	}
}

func TestClientListToolsEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tools":[]}`))
	}))
	defer srv.Close()
	_, err := httpplugin.NewClient(srv.URL).ListTools(context.Background())
	if !errors.Is(err, httpplugin.ErrInvalidPlugin) {
		t.Fatalf("err=%v", err)
	}
}

func TestClientListToolsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"d"}]}`))
	}))
	defer srv.Close()
	_, err := httpplugin.NewClient(srv.URL).ListTools(context.Background())
	if !errors.Is(err, httpplugin.ErrInvalidPlugin) {
		t.Fatalf("err=%v", err)
	}
}

func TestClientInvokeProtocolHeadersNotOverwritten(t *testing.T) {
	var gotProto, gotRun string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotProto = r.Header.Get("X-Baize-Protocol")
		gotRun = r.Header.Get("X-Baize-Run-Id")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":{},"is_error":false}`))
	}))
	defer srv.Close()
	_, err := httpplugin.NewClient(srv.URL).Invoke(context.Background(), "echo", nil, httpplugin.InvokeMeta{
		RunID: "run_1",
		Headers: map[string]string{
			"X-Baize-Protocol": "override",
			"X-Baize-Run-Id":   "override_run",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotProto != "v0" || gotRun != "run_1" {
		t.Fatalf("proto=%q run=%q", gotProto, gotRun)
	}
}

func TestClientInvokeHTTPErrorWithErrorJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"bad_request","message":"nope","retryable":false}}`))
	}))
	defer srv.Close()
	out, err := httpplugin.NewClient(srv.URL).Invoke(context.Background(), "echo", nil, httpplugin.InvokeMeta{})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Fatalf("want is_error out=%+v", out)
	}
	if out.Content["message"] != "nope" {
		t.Fatalf("content=%+v", out.Content)
	}
}
