package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestHostedLoginOAuthRedirectAndCallback(t *testing.T) {
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "atk", "token_type": "Bearer"})
	}))
	defer tok.Close()

	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"})
	srv := NewServer(st, tool.NewRegistry(), nil)
	srv.CallbackPublicBase = "http://127.0.0.1:8080"

	id, csrf, err := srv.HostedSessions.Issue(hostedlogin.Session{
		Mode: hostedlogin.ModeOAuth, IdentityID: "mei_1",
		PublicOrigin: "http://127.0.0.1:8080",
		OAuth: &store.HostedLoginOAuth{
			AuthorizeURL: "https://idp.example/authorize",
			TokenURL:     tok.URL,
			ClientID:     "cid",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/"+id, nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusFound {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	loc := rr.Header().Get("Location")
	if !strings.Contains(loc, "https://idp.example/authorize") || !strings.Contains(loc, "state="+csrf) {
		t.Fatalf("Location=%q", loc)
	}

	rr2 := httptest.NewRecorder()
	cb := "/v0/auth/hosted/" + id + "/oauth/callback?code=c1&state=" + url.QueryEscape(csrf)
	req2 := httptest.NewRequest(http.MethodGet, cb, nil)
	srv.Handler().ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("callback code=%d body=%s", rr2.Code, rr2.Body.String())
	}
	body, _ := io.ReadAll(rr2.Body)
	if !strings.Contains(string(body), "Signed in") {
		t.Fatalf("%s", body)
	}
	got, err := st.GetMCPExportIdentity("mei_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Headers["Authorization"] != "Bearer atk" {
		t.Fatalf("%+v", got.Headers)
	}
}

func TestHostedLoginPageIncludesQR(t *testing.T) {
	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"})
	srv := NewServer(st, tool.NewRegistry(), nil)
	id, _, err := srv.HostedSessions.Issue(hostedlogin.Session{
		Mode: hostedlogin.ModeTokenPaste, IdentityID: "mei_1",
		PublicOrigin: "http://127.0.0.1:8080",
		Fields:       []store.HostedLoginField{{Name: "token", Arg: "token", Input: "password"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/"+id, nil)
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("code=%d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "data:image/png;base64,") {
		t.Fatal("expected QR image")
	}
}
