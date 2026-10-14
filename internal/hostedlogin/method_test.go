package hostedlogin

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestMaterializeSessionFromMethod(t *testing.T) {
	cfg := store.HostedLoginConfig{
		Enabled: true, DefaultMethodID: "pwd",
		Methods: []store.HostedLoginMethod{{
			ID: "pwd", Mode: ModeTokenPaste,
			Fields: DefaultTokenFields(),
		}},
	}
	seed, err := MaterializeSession(cfg, "pwd", Session{ConnectorID: "mall"})
	if err != nil || seed.Mode != ModeTokenPaste {
		t.Fatalf("%+v %v", seed, err)
	}
	if seed.MethodID != "pwd" {
		t.Fatalf("method_id=%q", seed.MethodID)
	}
	if seed.ConnectorID != "mall" {
		t.Fatalf("connector=%q", seed.ConnectorID)
	}
}

func TestResolveMethodRequiresID(t *testing.T) {
	_, err := ResolveMethod(store.HostedLoginConfig{
		Enabled: true, DefaultMethodID: "pwd",
		Methods: []store.HostedLoginMethod{{ID: "pwd", Mode: ModeBasic, Fields: DefaultBasicFields()}},
	}, "")
	if err == nil {
		t.Fatal("want error")
	}
}

func TestResolveMethodUnknown(t *testing.T) {
	_, err := ResolveMethod(store.HostedLoginConfig{
		Methods: []store.HostedLoginMethod{{ID: "a", Mode: ModeTokenPaste, Fields: DefaultTokenFields()}},
	}, "missing")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("err=%v", err)
	}
}

func TestSwitchMethodUpdatesSession(t *testing.T) {
	st := store.NewMemory()
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled: true, DefaultMethodID: "pwd",
				Methods: []store.HostedLoginMethod{
					{ID: "pwd", Mode: ModeTokenPaste, Fields: DefaultTokenFields()},
					{ID: "basic", Mode: ModeBasic, Fields: DefaultBasicFields()},
				},
			},
		},
	})
	sessions := NewMemorySessions(0)
	id, csrf, err := sessions.Issue(mustMaterialize(t, st, "pwd"))
	if err != nil {
		t.Fatal(err)
	}
	err = SwitchMethod(SwitchMethodDeps{Sessions: sessions, Store: st}, id, csrf, "basic")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := sessions.Get(id)
	if !ok || got.MethodID != "basic" || got.Mode != ModeBasic {
		t.Fatalf("%+v ok=%v", got, ok)
	}
}

func mustMaterialize(t *testing.T, st store.Store, methodID string) Session {
	t.Helper()
	conn, err := st.GetConnector("mall")
	if err != nil {
		t.Fatal(err)
	}
	cfg := conn.Auth.HostedLogin
	seed, err := MaterializeSession(cfg, methodID, Session{ConnectorID: "mall", IdentityID: "mei_1", ExportKeyID: "k1"})
	if err != nil {
		t.Fatal(err)
	}
	return seed
}
