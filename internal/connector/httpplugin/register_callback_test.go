package httpplugin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/connector/httpplugin"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/plugincallback"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// stubSidecar returns the decoded request body of the last /invoke hit.
func stubSidecar(t *testing.T) (*httptest.Server, *map[string]any) {
	t.Helper()
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		case r.URL.Path == "/v0/tools" && r.Method == http.MethodGet:
			_, _ = w.Write([]byte(`{"tools":[{"name":"echo","description":"echo"}]}`))
		case strings.HasSuffix(r.URL.Path, "/invoke"):
			gotBody = map[string]any{}
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			_, _ = w.Write([]byte(`{"content":{"ok":true},"is_error":false}`))
		default:
			http.NotFound(w, r)
		}
	}))
	return srv, &gotBody
}

func registerWithCallback(t *testing.T, srv *httptest.Server, opts httpplugin.RegisterOpts) *tool.Registry {
	t.Helper()
	st := store.NewMemory()
	reg := tool.NewRegistry()
	if _, _, err := httpplugin.RegisterWithOpts(st, reg, opts); err != nil {
		t.Fatal(err)
	}
	return reg
}

func ctxMap(body *map[string]any) map[string]any {
	if body == nil || *body == nil {
		return nil
	}
	ctx, _ := (*body)["context"].(map[string]any)
	return ctx
}

func TestRegisterInjectsCallbackEventURL(t *testing.T) {
	srv, gotBody := stubSidecar(t)
	defer srv.Close()
	publicBase := "https://runtime.example/"
	reg := registerWithCallback(t, srv, httpplugin.RegisterOpts{
		ID:                 "side",
		BaseURL:            srv.URL,
		CallbackSigner:     httpplugin.CallbackSigner(plugincallback.Issue),
		CallbackSecret:     []byte("s3cret"),
		CallbackPublicBase: publicBase, // trailing slash on purpose
		CallbackTTL:        30 * time.Minute,
	})
	ctx := identity.WithRunID(context.Background(), "run_42")
	if _, isErr, err := reg.Invoke(ctx, "echo", nil); err != nil || isErr {
		t.Fatalf("invoke err=%v isErr=%v", err, isErr)
	}
	c := ctxMap(gotBody)
	urls, _ := c["callback_urls"].(map[string]any)
	if urls == nil {
		t.Fatalf("callback_urls missing: %+v", c)
	}
	event, _ := urls["event"].(string)
	wantPrefix := "https://runtime.example/v0/runs/run_42/plugin-callbacks?token="
	if !strings.HasPrefix(event, wantPrefix) {
		t.Fatalf("event=%q want prefix %q", event, wantPrefix)
	}
	if token := strings.TrimPrefix(event, wantPrefix); token == "" {
		t.Fatalf("token empty in %q", event)
	}
}

func TestRegisterOmitsCallbackURLWithoutRunID(t *testing.T) {
	srv, gotBody := stubSidecar(t)
	defer srv.Close()
	reg := registerWithCallback(t, srv, httpplugin.RegisterOpts{
		ID:                 "side",
		BaseURL:            srv.URL,
		CallbackSigner:     httpplugin.CallbackSigner(plugincallback.Issue),
		CallbackSecret:     []byte("s3cret"),
		CallbackPublicBase: "https://runtime.example",
	})
	// no WithRunID
	if _, isErr, err := reg.Invoke(context.Background(), "echo", nil); err != nil || isErr {
		t.Fatalf("invoke err=%v isErr=%v", err, isErr)
	}
	if c := ctxMap(gotBody); c != nil {
		if _, ok := c["callback_urls"]; ok {
			t.Fatalf("callback_urls must be absent without runID: %+v", c)
		}
	}
}

func TestRegisterOmitsCallbackURLWithoutSignerOrSecretOrPublicBase(t *testing.T) {
	cases := []struct {
		name string
		opts httpplugin.RegisterOpts
	}{
		{"no signer", httpplugin.RegisterOpts{
			ID: "side", BaseURL: "",
			CallbackSecret:     []byte("s3cret"),
			CallbackPublicBase: "https://runtime.example",
		}},
		{"no secret", httpplugin.RegisterOpts{
			ID:                 "side",
			CallbackSigner:     httpplugin.CallbackSigner(plugincallback.Issue),
			CallbackPublicBase: "https://runtime.example",
		}},
		{"no publicbase", httpplugin.RegisterOpts{
			ID:             "side",
			CallbackSigner: httpplugin.CallbackSigner(plugincallback.Issue),
			CallbackSecret: []byte("s3cret"),
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, gotBody := stubSidecar(t)
			defer srv.Close()
			tc.opts.BaseURL = srv.URL
			reg := registerWithCallback(t, srv, tc.opts)
			ctx := identity.WithRunID(context.Background(), "run_7")
			if _, isErr, err := reg.Invoke(ctx, "echo", nil); err != nil || isErr {
				t.Fatalf("invoke err=%v isErr=%v", err, isErr)
			}
			if c := ctxMap(gotBody); c != nil {
				if _, ok := c["callback_urls"]; ok {
					t.Fatalf("callback_urls must be absent (%s): %+v", tc.name, c)
				}
			}
		})
	}
}

func TestRegisterCallbackURLTokenVerifies(t *testing.T) {
	srv, gotBody := stubSidecar(t)
	defer srv.Close()
	const secret = "topsecret"
	reg := registerWithCallback(t, srv, httpplugin.RegisterOpts{
		ID:                 "side",
		BaseURL:            srv.URL,
		CallbackSigner:     httpplugin.CallbackSigner(plugincallback.Issue),
		CallbackSecret:     []byte(secret),
		CallbackPublicBase: "https://runtime.example",
	})
	ctx := identity.WithRunID(context.Background(), "run_99")
	if _, _, err := reg.Invoke(ctx, "echo", nil); err != nil {
		t.Fatal(err)
	}
	c := ctxMap(gotBody)
	urls, _ := c["callback_urls"].(map[string]any)
	event, _ := urls["event"].(string)
	token := strings.TrimPrefix(event, "https://runtime.example/v0/runs/run_99/plugin-callbacks?token=")
	if err := plugincallback.Verify([]byte(secret), "run_99", token, time.Now()); err != nil {
		t.Fatalf("token did not verify: %v", err)
	}
	// wrong runID must fail (token bound to run_99)
	if err := plugincallback.Verify([]byte(secret), "run_other", token, time.Now()); err == nil {
		t.Fatal("token must not verify for a different runID")
	}
}
