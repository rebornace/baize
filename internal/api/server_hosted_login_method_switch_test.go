package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestHostedLoginMethodSwitchUpdatesFormFields(t *testing.T) {
	st := store.NewMemory()
	if err := st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"}); err != nil {
		t.Fatal(err)
	}
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled: true, DefaultMethodID: "pwd",
				Methods: []store.HostedLoginMethod{
					{ID: "pwd", Mode: hostedlogin.ModeTokenPaste, Fields: hostedlogin.DefaultTokenFields()},
					{ID: "basic", Mode: hostedlogin.ModeBasic, Fields: hostedlogin.DefaultBasicFields()},
				},
			},
		},
	})
	srv := NewServer(st, tool.NewRegistry(), nil)

	id, csrf, err := issueHostedSessionForMethod(t, srv, st, "pwd", hostedlogin.Session{
		ConnectorID: "mall", IdentityID: "mei_1", ExportKeyID: "k1",
		PublicOrigin: "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/"+id, nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("initial GET code=%d body=%s", rr.Code, rr.Body.String())
	}
	initial := rr.Body.String()
	if !strings.Contains(initial, `name="token"`) {
		t.Fatalf("expected token field: %s", initial)
	}
	if strings.Contains(initial, `name="username"`) {
		t.Fatal("username field should not appear before switch")
	}

	switchForm := url.Values{"csrf": {csrf}, "method_id": {"basic"}}
	req = httptest.NewRequest(http.MethodPost, "/v0/auth/hosted/"+id+"/method", strings.NewReader(switchForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("switch POST code=%d body=%s", rr.Code, rr.Body.String())
	}
	wantLoc := "/v0/auth/hosted/" + id
	if got := rr.Header().Get("Location"); got != wantLoc {
		t.Fatalf("Location=%q want %q", got, wantLoc)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/"+id, nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("after switch GET code=%d body=%s", rr.Code, rr.Body.String())
	}
	after := rr.Body.String()
	if !strings.Contains(after, `name="username"`) || !strings.Contains(after, `name="password"`) {
		t.Fatalf("expected basic fields: %s", after)
	}
	if strings.Contains(after, `name="token"`) {
		t.Fatal("token field should not appear after switch to basic")
	}
	if !strings.Contains(after, csrf) {
		t.Fatal("csrf unchanged after method switch")
	}
}

func TestHostedLoginMethodSwitchToOAuthRedirectsOnGET(t *testing.T) {
	st := store.NewMemory()
	if err := st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"}); err != nil {
		t.Fatal(err)
	}
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			HostedLogin: store.HostedLoginConfig{
				Enabled: true, DefaultMethodID: "pwd",
				Methods: []store.HostedLoginMethod{
					{ID: "pwd", Mode: hostedlogin.ModeTokenPaste, Fields: hostedlogin.DefaultTokenFields()},
					{
						ID: "oauth", Mode: hostedlogin.ModeOAuth,
						OAuth: &store.HostedLoginOAuth{
							AuthorizeURL: "https://idp.example/authorize",
							TokenURL:     "https://idp.example/token",
							ClientID:     "cid",
						},
					},
				},
			},
		},
	})
	srv := NewServer(st, tool.NewRegistry(), nil)
	srv.CallbackPublicBase = "http://127.0.0.1:8080"

	id, csrf, err := issueHostedSessionForMethod(t, srv, st, "pwd", hostedlogin.Session{
		ConnectorID: "mall", IdentityID: "mei_1",
		PublicOrigin: "http://127.0.0.1:8080",
	})
	if err != nil {
		t.Fatal(err)
	}

	switchForm := url.Values{"csrf": {csrf}, "method_id": {"oauth"}}
	req := httptest.NewRequest(http.MethodPost, "/v0/auth/hosted/"+id+"/method", strings.NewReader(switchForm.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusSeeOther {
		t.Fatalf("switch POST code=%d body=%s", rr.Code, rr.Body.String())
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/"+id, nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("GET after oauth switch code=%d body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "https://idp.example/authorize") || !strings.Contains(loc, "state="+csrf) {
		t.Fatalf("Location=%q", loc)
	}
}

func issueHostedSessionForMethod(t *testing.T, srv *Server, st store.Store, methodID string, seed hostedlogin.Session) (id, csrf string, err error) {
	t.Helper()
	conn, err := st.GetConnector(seed.ConnectorID)
	if err != nil {
		return "", "", err
	}
	materialized, err := hostedlogin.MaterializeSession(conn.Auth.HostedLogin, methodID, seed)
	if err != nil {
		return "", "", err
	}
	return srv.HostedSessions.Issue(materialized)
}
