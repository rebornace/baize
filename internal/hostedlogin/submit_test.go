package hostedlogin

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

type fakeReg struct {
	lastName string
	lastArgs map[string]any
	content  map[string]any
	isError  bool
	err      error
}

func (f *fakeReg) Invoke(ctx context.Context, name string, args map[string]any) (map[string]any, bool, error) {
	f.lastName = name
	f.lastArgs = args
	return f.content, f.isError, f.err
}

func TestSubmitPasswordUpdatesIdentity(t *testing.T) {
	st := store.NewMemory()
	identityID := "mei_1"
	if err := st.UpsertMCPExportIdentity(store.MCPExportIdentity{
		ID:      identityID,
		Name:    "Ops",
		Scheme:  "Bearer",
		Headers: map[string]string{"Authorization": "Bearer old"},
	}); err != nil {
		t.Fatal(err)
	}
	st.UpsertConnector(store.Connector{
		ID:   "mall",
		Type: "openapi",
		Auth: store.ConnectorAuth{
			Capture: store.CaptureAuth{
				ToolNameGlob:   "*login*",
				TokenJSONPaths: []string{"accessToken"},
				HeaderTemplate: "Bearer {{token}}",
			},
			HostedLogin: store.HostedLoginConfig{
				Enabled:         true,
				DefaultMethodID: "default",
				Methods: []store.HostedLoginMethod{{
					ID: "default", LoginTool: "mall.login", Mode: ModePasswordForm,
				}},
			},
		},
	})

	sessions := NewMemorySessions(time.Minute)
	id, csrf, err := sessions.Issue(Session{
		ExportKeyID: "k1",
		ConnectorID: "mall",
		LoginTool:   "mall.login",
		Mode:        ModePasswordForm,
		IdentityID:  identityID,
		Fields: []store.HostedLoginField{
			{Name: "user", Arg: "username", Input: "text"},
			{Name: "pass", Arg: "password", Input: "password"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	reg := &fakeReg{content: map[string]any{"accessToken": "tok_abc_long", "email": "u@x"}}
	ids := identity.NewMemoryStore()
	err = Submit(context.Background(), SubmitDeps{
		Sessions:   sessions,
		Store:      st,
		Registry:   reg,
		Identities: ids,
	}, id, csrf, map[string]string{"user": "alice", "pass": "secret"})
	if err != nil {
		t.Fatal(err)
	}
	if reg.lastName != "mall.login" {
		t.Fatalf("tool=%q", reg.lastName)
	}
	if reg.lastArgs["username"] != "alice" || reg.lastArgs["password"] != "secret" {
		t.Fatalf("args=%v", reg.lastArgs)
	}
	got, err := st.GetMCPExportIdentity(identityID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Headers["Authorization"] != "Bearer tok_abc_long" {
		t.Fatalf("headers=%v", got.Headers)
	}
	if _, ok := sessions.Get(id); ok {
		t.Fatal("session should be consumed")
	}
}

func TestSubmitConversationUpdatesIdentityStore(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			Capture: store.CaptureAuth{
				TokenJSONPaths: []string{"accessToken"},
				HeaderTemplate: "Bearer {{token}}",
			},
		},
	})
	sessions := NewMemorySessions(time.Minute)
	id, csrf, err := sessions.Issue(Session{
		ConnectorID:    "mall",
		LoginTool:      "mall.login",
		Mode:           ModePasswordForm,
		ConversationID: "conv_chat_1",
		Fields: []store.HostedLoginField{
			{Name: "user", Arg: "username", Input: "text"},
			{Name: "pass", Arg: "password", Input: "password"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ids := identity.NewMemoryStore()
	reg := &fakeReg{content: map[string]any{"accessToken": "tok_conv_long", "email": "u@x"}}
	if err := Submit(context.Background(), SubmitDeps{
		Sessions: sessions, Store: st, Registry: reg, Identities: ids,
	}, id, csrf, map[string]string{"user": "a", "pass": "b"}); err != nil {
		t.Fatal(err)
	}
	list := ids.List(identity.Scope("conv_chat_1"))
	if len(list) == 0 {
		t.Fatal("expected conversation identity")
	}
	if list[0].CredentialHeaders["Authorization"] != "Bearer tok_conv_long" {
		t.Fatalf("headers=%v", list[0].CredentialHeaders)
	}
}

func TestSubmitBasicDirectHeaders(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"})
	st.UpsertConnector(store.Connector{ID: "mall", Type: "openapi"})
	sessions := NewMemorySessions(time.Minute)
	id, csrf, err := sessions.Issue(Session{
		ExportKeyID: "k1", ConnectorID: "mall", Mode: ModeBasic, IdentityID: "mei_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Submit(context.Background(), SubmitDeps{
		Sessions: sessions, Store: st, Identities: identity.NewMemoryStore(),
	}, id, csrf, map[string]string{"username": "alice", "password": "s3cret"}); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetMCPExportIdentity("mei_1")
	if !strings.HasPrefix(got.Headers["Authorization"], "Basic ") {
		t.Fatalf("headers=%v", got.Headers)
	}
}

func TestSubmitHeaderPairs(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"})
	st.UpsertConnector(store.Connector{ID: "mall", Type: "openapi"})
	sessions := NewMemorySessions(time.Minute)
	id, csrf, err := sessions.Issue(Session{
		ExportKeyID: "k1", ConnectorID: "mall", Mode: ModeHeaderPairs, IdentityID: "mei_1",
		Fields: []store.HostedLoginField{{Name: "Authorization", Arg: "Authorization", Input: "password"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := Submit(context.Background(), SubmitDeps{
		Sessions: sessions, Store: st,
	}, id, csrf, map[string]string{"Authorization": "Bearer tok_header_long"}); err != nil {
		t.Fatal(err)
	}
	got, _ := st.GetMCPExportIdentity("mei_1")
	if got.Headers["Authorization"] != "Bearer tok_header_long" {
		t.Fatalf("headers=%v", got.Headers)
	}
}

func TestSubmitInvalidCSRF(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"})
	st.UpsertConnector(store.Connector{ID: "mall", Type: "openapi"})
	sessions := NewMemorySessions(time.Minute)
	id, _, err := sessions.Issue(Session{
		ExportKeyID: "k1", ConnectorID: "mall", LoginTool: "mall.login",
		Mode: ModePasswordForm, IdentityID: "mei_1",
		Fields: []store.HostedLoginField{{Name: "pass", Arg: "password", Input: "password"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = Submit(context.Background(), SubmitDeps{
		Sessions: sessions, Store: st, Registry: &fakeReg{content: map[string]any{"accessToken": "x"}},
	}, id, "wrong", map[string]string{"pass": "x"})
	if err == nil || !strings.Contains(err.Error(), "csrf") {
		t.Fatalf("err=%v", err)
	}
}
