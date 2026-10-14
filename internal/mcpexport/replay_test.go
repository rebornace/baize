package mcpexport

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestAuthExpiredForCallReturnsURLWithoutBlockingWhenNoElicitation(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled: true, DefaultMethodID: "default",
				Methods: []store.HostedLoginMethod{{
					ID: "default", Mode: hostedlogin.ModeTokenPaste,
					Fields: []store.HostedLoginField{{Name: "token", Arg: "token", Input: "password"}},
				}},
			},
		},
	})
	idRow := store.MCPExportIdentity{ID: "mei_1", Name: "Ops"}
	_ = st.UpsertMCPExportIdentity(idRow)
	key := store.MCPExportKey{ID: "k1", Name: "k", IdentityID: "mei_1", Prefix: "bz_", KeyHash: "hash1"}
	if err := st.InsertMCPExportKey(key); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	reg := tool.NewRegistry()
	reg.Register("mall.orders", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		n := calls.Add(1)
		return map[string]any{"ok": true, "n": float64(n)}, false, nil
	})

	base := hostedlogin.NewMemorySessions(time.Minute)
	rec := &recordingSessions{inner: base}
	deps := ServerDeps{
		Store:          st,
		Registry:       reg,
		Identities:     identity.NewMemoryStore(),
		HostedSessions: rec,
		HostedWaiter:   hostedlogin.NewMemoryWaiter(),
		PublicBaseURL:  "http://127.0.0.1:8080",
		Enabled:        true,
	}
	toolRow := store.Tool{Name: "mall.orders", ConnectorID: "mall", Method: "GET"}
	auth := exportAuth{Key: key, Identity: idRow}

	// nil CallToolRequest ⇒ no client elicitation ⇒ return URL immediately
	// (Doubao-like agents). Do not block on HostedWaiter.
	res, err := authExpiredForCall(context.Background(), deps, auth, toolRow, nil, "mall.orders", map[string]any{"id": 1})
	if err != nil {
		t.Fatalf("authExpiredForCall err=%v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("result=%+v", res)
	}
	if calls.Load() != 0 {
		t.Fatalf("must not replay without elicitation client, calls=%d", calls.Load())
	}
	if rec.lastID() == "" {
		t.Fatal("session should still be issued")
	}
	text := callToolText(res)
	if !strings.Contains(text, "/v0/auth/hosted/") {
		t.Fatalf("want hosted url in text %q", text)
	}
	sc, ok := res.StructuredContent.(map[string]any)
	if !ok || sc["code"] != HostedLoginRequiredCode {
		t.Fatalf("structured=%+v", res.StructuredContent)
	}
	url, _ := sc["hosted_login_url"].(string)
	if !strings.Contains(url, "/v0/auth/hosted/") {
		t.Fatalf("hosted_login_url=%q", url)
	}
}

func TestAuthExpiredForCallTimeoutFallsBack(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled: true, DefaultMethodID: "default",
				Methods: []store.HostedLoginMethod{{
					ID: "default", Mode: hostedlogin.ModeTokenPaste,
					Fields: []store.HostedLoginField{{Name: "token", Arg: "token", Input: "password"}},
				}},
			},
		},
	})
	idRow := store.MCPExportIdentity{ID: "mei_1", Name: "Ops"}
	if err := st.UpsertMCPExportIdentity(idRow); err != nil {
		t.Fatal(err)
	}
	key := store.MCPExportKey{ID: "k1", Name: "k", IdentityID: "mei_1", Prefix: "bz_", KeyHash: "hash-timeout"}

	deps := ServerDeps{
		Store:          st,
		Registry:       tool.NewRegistry(),
		Identities:     identity.NewMemoryStore(),
		HostedSessions: hostedlogin.NewMemorySessions(time.Minute),
		HostedWaiter:   hostedlogin.NewMemoryWaiter(),
		PublicBaseURL:  "http://127.0.0.1:8080",
		Enabled:        true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	res, err := authExpiredForCall(ctx, deps, exportAuth{Key: key, Identity: idRow}, store.Tool{
		Name: "mall.orders", ConnectorID: "mall",
	}, nil, "mall.orders", nil)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if res == nil || !res.IsError {
		t.Fatalf("%+v", res)
	}
	text := callToolText(res)
	if !strings.Contains(text, "/v0/auth/hosted/") {
		t.Fatalf("want hosted url in %q", text)
	}
	sc, ok := res.StructuredContent.(map[string]any)
	if !ok {
		t.Fatalf("structured=%T", res.StructuredContent)
	}
	if sc["code"] != HostedLoginRequiredCode {
		t.Fatalf("code=%v", sc["code"])
	}
	url, _ := sc["hosted_login_url"].(string)
	if !strings.Contains(url, "/v0/auth/hosted/") {
		t.Fatalf("hosted_login_url=%q", url)
	}
}

func callToolText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

type recordingSessions struct {
	inner hostedlogin.Sessions
	last  string
}

func (r *recordingSessions) Issue(seed hostedlogin.Session) (id, csrf string, err error) {
	id, csrf, err = r.inner.Issue(seed)
	if err == nil && id != "" {
		r.last = id
	}
	return id, csrf, err
}

func (r *recordingSessions) Get(id string) (hostedlogin.Session, bool) {
	return r.inner.Get(id)
}

func (r *recordingSessions) Replace(id string, sess hostedlogin.Session) bool {
	return r.inner.Replace(id, sess)
}

func (r *recordingSessions) Delete(id string) { r.inner.Delete(id) }

func (r *recordingSessions) lastID() string { return r.last }
