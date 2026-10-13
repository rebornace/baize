package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestHostedLoginPagePublicAndSubmit(t *testing.T) {
	st := store.NewMemory()
	reg := toolRegistryWithLogin(t)
	if err := st.UpsertMCPExportIdentity(store.MCPExportIdentity{
		ID: "mei_1", Name: "Ops", Scheme: "Bearer",
		Headers: map[string]string{"Authorization": "Bearer old_token_xx"},
	}); err != nil {
		t.Fatal(err)
	}
	st.UpsertConnector(store.Connector{
		ID: "mall", Type: "openapi",
		Auth: store.ConnectorAuth{
			Capture: store.CaptureAuth{
				ToolNameGlob: "*login*", TokenJSONPaths: []string{"accessToken"},
				HeaderTemplate: "Bearer {{token}}",
			},
			HostedLogin: store.HostedLoginRecipe{
				Enabled: true, LoginTool: "mall.login", Mode: hostedlogin.ModePasswordForm,
			},
		},
	})
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.OperatorToken = "op"
	srv.AdminToken = "adm"

	id, csrf, err := srv.HostedSessions.Issue(hostedlogin.Session{
		ExportKeyID: "k1", ConnectorID: "mall", LoginTool: "mall.login",
		Mode: hostedlogin.ModePasswordForm, IdentityID: "mei_1",
		Fields: []store.HostedLoginField{
			{Name: "user", Arg: "username", Input: "text"},
			{Name: "pass", Arg: "password", Input: "password"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// GET is public (gate on, no Authorization)
	req := httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/"+id, nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET code=%d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("content-type=%q", ct)
	}
	if rr.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("cache=%q", rr.Header().Get("Cache-Control"))
	}
	if !strings.Contains(rr.Body.String(), csrf) {
		t.Fatal("missing csrf in page")
	}

	form := url.Values{"csrf": {csrf}, "user": {"alice"}, "pass": {"secret"}}
	req = httptest.NewRequest(http.MethodPost, "/v0/auth/hosted/"+id, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("POST code=%d body=%s", rr.Code, rr.Body.String())
	}
	got, err := st.GetMCPExportIdentity("mei_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Headers["Authorization"] != "Bearer fresh_token_ok" {
		t.Fatalf("headers=%v", got.Headers)
	}

	// bad csrf
	id2, csrf2, err := srv.HostedSessions.Issue(hostedlogin.Session{
		ExportKeyID: "k1", ConnectorID: "mall", LoginTool: "mall.login",
		Mode: hostedlogin.ModePasswordForm, IdentityID: "mei_1",
		Fields: []store.HostedLoginField{{Name: "pass", Arg: "password", Input: "password"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	form = url.Values{"csrf": {"nope"}, "pass": {"x"}}
	req = httptest.NewRequest(http.MethodPost, "/v0/auth/hosted/"+id2, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr = httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad csrf code=%d", rr.Code)
	}
	_ = csrf2
}

func toolRegistryWithLogin(t *testing.T) *tool.Registry {
	t.Helper()
	reg := tool.NewRegistry()
	reg.Register("mall.login", func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		return map[string]any{"accessToken": "fresh_token_ok", "email": "a@x"}, false, nil
	})
	return reg
}
