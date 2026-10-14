package hostedlogin_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
)

func TestValidateRecipeOAuth(t *testing.T) {
	if err := hostedlogin.ValidateMethod(store.HostedLoginMethod{
		Mode: hostedlogin.ModeOAuth,
	}); err == nil {
		t.Fatal("want error")
	}
	if err := hostedlogin.ValidateMethod(store.HostedLoginMethod{
		Mode: hostedlogin.ModeOAuth,
		OAuth: &store.HostedLoginOAuth{
			AuthorizeURL: "https://idp.example/authorize",
			TokenURL:     "https://idp.example/token",
			ClientID:     "cid",
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func TestOAuthAuthorizeURL(t *testing.T) {
	u, err := hostedlogin.OAuthAuthorizeURL(hostedlogin.Session{
		ID: "sid", CSRF: "csrf", PublicOrigin: "http://127.0.0.1:8080",
		OAuth: &store.HostedLoginOAuth{
			AuthorizeURL: "https://idp.example/authorize",
			ClientID:     "cid",
			Scopes:       []string{"openid", "profile"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("client_id") != "cid" || q.Get("state") != "csrf" || q.Get("response_type") != "code" {
		t.Fatalf("%v", q)
	}
	if !strings.Contains(q.Get("redirect_uri"), "/v0/auth/hosted/sid/oauth/callback") {
		t.Fatalf("redirect_uri=%q", q.Get("redirect_uri"))
	}
	if q.Get("scope") != "openid profile" {
		t.Fatalf("scope=%q", q.Get("scope"))
	}
}

func TestCompleteOAuthPersistsBearer(t *testing.T) {
	tok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("code") != "authcode" {
			t.Fatalf("code=%q", r.Form.Get("code"))
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"access_token": "tok-1", "token_type": "Bearer"})
	}))
	defer tok.Close()

	st := store.NewMemory()
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"})
	sessions := hostedlogin.NewMemorySessions(time.Minute)
	waiter := hostedlogin.NewMemoryWaiter()
	id, csrf, err := sessions.Issue(hostedlogin.Session{
		Mode: hostedlogin.ModeOAuth, IdentityID: "mei_1",
		PublicOrigin: "http://127.0.0.1:8080",
		OAuth: &store.HostedLoginOAuth{
			AuthorizeURL: "https://idp.example/authorize",
			TokenURL:     tok.URL,
			ClientID:     "cid",
			ClientSecret: "sec",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- waiter.Await(context.Background(), id) }()
	time.Sleep(20 * time.Millisecond)

	err = hostedlogin.CompleteOAuth(context.Background(), hostedlogin.SubmitDeps{
		Sessions: sessions, Store: st, Identities: identity.NewMemoryStore(), Waiter: waiter,
	}, id, csrf, "authcode")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("waiter timeout")
	}
	got, err := st.GetMCPExportIdentity("mei_1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Headers["Authorization"] != "Bearer tok-1" {
		t.Fatalf("%+v", got.Headers)
	}
}
