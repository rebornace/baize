package connector_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func newHTTPCaptureLoginServer(t *testing.T, loginBody string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"login","description":"login"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			_, _ = w.Write([]byte(loginBody))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestApplyHTTPCaptureNoneDisables(t *testing.T) {
	srv := newHTTPCaptureLoginServer(t, `{"content":{"accessToken":"tok-none","email":"a@x.com"},"is_error":false}`)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "side", Type: "http", BaseURL: srv.URL,
		RequireLogin: ptr([]string{}),
		Auth: store.ConnectorAuth{
			Mode: "static",
			Capture: store.CaptureAuth{
				ToolNameGlob:   "__none__",
				TokenJSONPaths: []string{"accessToken"},
				LabelJSONPaths: []string{"email"},
				HeaderTemplate: "Bearer {{token}}",
			},
		},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	ctx := identity.WithConversationID(context.Background(), "conv_none")
	_, isErr, invErr := reg.Invoke(ctx, "login", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("invoke login: isErr=%v err=%v", isErr, invErr)
	}
	if len(ids.List("conv_none")) != 0 {
		t.Fatalf("expected no identity with __none__ glob, got %v", ids.List("conv_none"))
	}
}

func TestApplyHTTPCaptureSkipsWithoutConversation(t *testing.T) {
	srv := newHTTPCaptureLoginServer(t, `{"content":{"accessToken":"tok-noconv","email":"a@x.com"},"is_error":false}`)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "side", Type: "http", BaseURL: srv.URL,
		RequireLogin: ptr([]string{}),
		Auth: store.ConnectorAuth{
			Mode: "static",
			Capture: store.CaptureAuth{
				ToolNameGlob:   "*login*",
				TokenJSONPaths: []string{"accessToken"},
				LabelJSONPaths: []string{"email"},
				HeaderTemplate: "Bearer {{token}}",
			},
		},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	_, isErr, invErr := reg.Invoke(context.Background(), "login", map[string]any{})
	if invErr != nil || isErr {
		t.Fatalf("invoke login: isErr=%v err=%v", isErr, invErr)
	}
	if len(ids.List("")) != 0 {
		t.Fatalf("expected no identity without conversation, got %v", ids.List(""))
	}
	if len(ids.List("conv_any")) != 0 {
		t.Fatalf("expected no identity in any conversation, got %v", ids.List("conv_any"))
	}
}

func TestApplyHTTPCaptureSkipsOnToolError(t *testing.T) {
	srv := newHTTPCaptureLoginServer(t, `{"content":{},"is_error":true}`)

	st := store.NewMemory()
	reg := tool.NewRegistry()
	ids := identity.NewMemoryStore()
	if _, _, err := connector.Apply(connector.ApplyInput{
		Store: st, Registry: reg, Identities: ids,
		ID: "side", Type: "http", BaseURL: srv.URL,
		RequireLogin: ptr([]string{}),
		Auth: store.ConnectorAuth{
			Mode: "static",
			Capture: store.CaptureAuth{
				ToolNameGlob:   "*login*",
				TokenJSONPaths: []string{"accessToken"},
				LabelJSONPaths: []string{"email"},
				HeaderTemplate: "Bearer {{token}}",
			},
		},
	}); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	ctx := identity.WithConversationID(context.Background(), "conv_err")
	_, _, invErr := reg.Invoke(ctx, "login", map[string]any{})
	if invErr != nil {
		t.Fatalf("invoke login: err=%v", invErr)
	}
	if len(ids.List("conv_err")) != 0 {
		t.Fatalf("expected no identity on tool error, got %v", ids.List("conv_err"))
	}
}
