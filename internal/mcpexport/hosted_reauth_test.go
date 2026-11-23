package mcpexport

import (
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
)

func TestHostedReauthURL(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled: true, DefaultMethodID: "default",
				Methods: []store.HostedLoginMethod{{
					ID: "default", LoginTool: "mall.login", Mode: hostedlogin.ModePasswordForm,
					Fields: []store.HostedLoginField{
						{Name: "user", Arg: "username", Input: "text"},
						{Name: "pass", Arg: "password", Input: "password"},
					},
				}},
			},
		},
	})
	sessions := hostedlogin.NewMemorySessions(time.Minute)
	deps := ServerDeps{
		Store:          st,
		HostedSessions: sessions,
		PublicBaseURL:  "http://127.0.0.1:8080",
	}
	url, id, ok := hostedReauthURL(deps, exportAuth{
		Key:      store.MCPExportKey{ID: "k1"},
		Identity: store.MCPExportIdentity{ID: "mei_1", Name: "Ops"},
	}, store.Tool{Name: "mall.get", ConnectorID: "mall"})
	if !ok {
		t.Fatal("expected url")
	}
	if !strings.HasPrefix(url, "http://127.0.0.1:8080/v0/auth/hosted/") {
		t.Fatalf("url=%q", url)
	}
	if id == "" {
		t.Fatal("empty session id")
	}
	sess, ok := sessions.Get(id)
	if !ok {
		t.Fatal("session missing")
	}
	if sess.MethodID != "default" {
		t.Fatalf("method_id=%q", sess.MethodID)
	}
	remain := time.Until(sess.ExpiresAt)
	if remain < 4*time.Minute || remain > 6*time.Minute {
		t.Fatalf("MCP reauth TTL remain=%v want ~%v", remain, hostedlogin.MCPReauthSessionTTL)
	}
}

func TestHostedReauthURLDisabled(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{ID: "mall", Type: "openapi"})
	deps := ServerDeps{
		Store:          st,
		HostedSessions: hostedlogin.NewMemorySessions(time.Minute),
		PublicBaseURL:  "http://127.0.0.1:8080",
	}
	if _, _, ok := hostedReauthURL(deps, exportAuth{
		Key: store.MCPExportKey{ID: "k1"}, Identity: store.MCPExportIdentity{ID: "mei_1"},
	}, store.Tool{ConnectorID: "mall"}); ok {
		t.Fatal("expected disabled")
	}
}
