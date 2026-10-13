package mcpexport

import (
	"context"
	"encoding/json"
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

func TestAuthExpiredForCallReplaysAfterNotify(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginRecipe{
				Enabled: true, Mode: hostedlogin.ModeTokenPaste,
				Fields: []store.HostedLoginField{{Name: "token", Arg: "token", Input: "password"}},
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

	// Custom sessions that records the last issued id so the test can NotifyOK.
	base := hostedlogin.NewMemorySessions(time.Minute)
	rec := &recordingSessions{inner: base}
	waiter := hostedlogin.NewMemoryWaiter()
	deps := ServerDeps{
		Store:          st,
		Registry:       reg,
		Identities:     identity.NewMemoryStore(),
		HostedSessions: rec,
		HostedWaiter:   waiter,
		PublicBaseURL:  "http://127.0.0.1:8080",
		Enabled:        true,
	}
	toolRow := store.Tool{Name: "mall.orders", ConnectorID: "mall", Method: "GET"}
	auth := exportAuth{Key: key, Identity: idRow}

	done := make(chan *mcp.CallToolResult, 1)
	go func() {
		done <- authExpiredForCall(context.Background(), deps, auth, toolRow, nil, "mall.orders", map[string]any{"id": 1})
	}()

	deadline := time.Now().Add(2 * time.Second)
	var sid string
	for time.Now().Before(deadline) {
		sid = rec.lastID()
		if sid != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if sid == "" {
		t.Fatal("session not issued")
	}
	time.Sleep(20 * time.Millisecond) // let Await register
	waiter.NotifyOK(sid)

	select {
	case res := <-done:
		if res == nil || res.IsError {
			t.Fatalf("result=%+v", res)
		}
		text := ""
		if len(res.Content) > 0 {
			if tc, ok := res.Content[0].(*mcp.TextContent); ok {
				text = tc.Text
			}
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(text), &payload); err != nil {
			t.Fatalf("text=%q err=%v", text, err)
		}
		if payload["ok"] != true {
			t.Fatalf("%v", payload)
		}
		if calls.Load() != 1 {
			t.Fatalf("calls=%d", calls.Load())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for replay")
	}
}

func TestAuthExpiredForCallTimeoutFallsBack(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginRecipe{
				Enabled: true, Mode: hostedlogin.ModeTokenPaste,
				Fields: []store.HostedLoginField{{Name: "token", Arg: "token", Input: "password"}},
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
	res := authExpiredForCall(ctx, deps, exportAuth{Key: key, Identity: idRow}, store.Tool{
		Name: "mall.orders", ConnectorID: "mall",
	}, nil, "mall.orders", nil)
	if res == nil || !res.IsError {
		t.Fatalf("%+v", res)
	}
	text := ""
	if len(res.Content) > 0 {
		if tc, ok := res.Content[0].(*mcp.TextContent); ok {
			text = tc.Text
		}
	}
	if !strings.Contains(text, "/v0/auth/hosted/") {
		t.Fatalf("want hosted url in %q", text)
	}
}

type recordingSessions struct {
	inner  hostedlogin.Sessions
	last   string
	muLast chan struct{} // unused placeholder avoided; use atomic string via simple field+sync
	idCh   chan string
}

func (r *recordingSessions) Issue(seed hostedlogin.Session) (id, csrf string, err error) {
	id, csrf, err = r.inner.Issue(seed)
	if err == nil && id != "" {
		r.last = id
		if r.idCh != nil {
			select {
			case r.idCh <- id:
			default:
			}
		}
	}
	return id, csrf, err
}

func (r *recordingSessions) Get(id string) (hostedlogin.Session, bool) {
	return r.inner.Get(id)
}

func (r *recordingSessions) Delete(id string) { r.inner.Delete(id) }

func (r *recordingSessions) lastID() string { return r.last }
