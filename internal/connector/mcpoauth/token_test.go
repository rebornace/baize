package mcpoauth

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/settingscrypto"
)

func TestSealOpenTokenBundle(t *testing.T) {
	t.Setenv("BAIZE_SETTINGS_KEY", "test-settings-key-32bytes-ok!!")
	key, err := settingscrypto.KeyFromEnv()
	if err != nil || key == nil {
		t.Fatalf("KeyFromEnv: %v", err)
	}
	b := TokenBundle{
		AccessToken:  "at-1",
		RefreshToken: "rt-1",
		TokenType:    "Bearer",
		Scope:        "mcp",
		ExpiresAt:    time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}
	sealed, err := SealBundle(key, b)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(sealed, settingscrypto.Prefix) {
		t.Fatalf("sealed=%q want bz1 prefix", sealed)
	}
	got, err := OpenBundle(key, sealed)
	if err != nil {
		t.Fatal(err)
	}
	if got.AccessToken != b.AccessToken || got.RefreshToken != b.RefreshToken || got.Scope != b.Scope {
		t.Fatalf("roundtrip=%+v want %+v", got, b)
	}
	if !got.ExpiresAt.Equal(b.ExpiresAt) {
		t.Fatalf("expires_at=%v want %v", got.ExpiresAt, b.ExpiresAt)
	}

	t.Setenv("BAIZE_SETTINGS_KEY", "")
	_, err = SealBundle(nil, b)
	if !errors.Is(err, settingscrypto.ErrNoKey) {
		t.Fatalf("SealBundle without key: want ErrNoKey, got %v", err)
	}
}

func TestMergeHeadersOAuthWinsAuthorization(t *testing.T) {
	static := map[string]string{"Authorization": "Bearer static", "X-A": "1"}
	got := MergeHeaders(static, "access-xyz")
	if got["Authorization"] != "Bearer access-xyz" {
		t.Fatalf("Authorization=%q", got["Authorization"])
	}
	if got["X-A"] != "1" {
		t.Fatalf("X-A=%q", got["X-A"])
	}
	if static["Authorization"] != "Bearer static" {
		t.Fatal("MergeHeaders must not mutate static map")
	}
}

func TestRefreshIfNeeded(t *testing.T) {
	const newAccess = "fresh-access"
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  newAccess,
			"token_type":    "Bearer",
			"expires_in":    3600,
			"refresh_token": "new-refresh",
		})
	}))
	defer srv.Close()

	ctx := context.Background()
	expired := TokenBundle{
		AccessToken:  "old",
		RefreshToken: "rt-old",
		ExpiresAt:    time.Now().Add(-time.Minute),
	}
	refreshed, err := EnsureAccessToken(ctx, http.DefaultClient, srv.URL, "cid", "", expired, 0)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.AccessToken != newAccess {
		t.Fatalf("access=%q", refreshed.AccessToken)
	}
	if gotForm.Get("grant_type") != "refresh_token" || gotForm.Get("refresh_token") != "rt-old" {
		t.Fatalf("form=%v", gotForm)

	}

	_, err = EnsureAccessToken(ctx, http.DefaultClient, srv.URL, "cid", "", TokenBundle{
		AccessToken: "only",
		ExpiresAt:   time.Now().Add(-time.Minute),
	}, 0)
	if !errors.Is(err, ErrNeedsReauth) {
		t.Fatalf("want ErrNeedsReauth, got %v", err)
	}
}

func TestExchangeCode(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(body))
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "code-access",
			"token_type":    "Bearer",
			"expires_in":    7200,
			"refresh_token": "code-refresh",
			"scope":         "openid",
		})
	}))
	defer srv.Close()

	b, err := ExchangeCode(context.Background(), http.DefaultClient, srv.URL,
		"client-id", "", "auth-code", "https://app/cb", "verifier-xyz")
	if err != nil {
		t.Fatal(err)
	}
	if b.AccessToken != "code-access" || b.RefreshToken != "code-refresh" || b.Scope != "openid" {
		t.Fatalf("bundle=%+v", b)
	}
	if gotForm.Get("grant_type") != "authorization_code" {
		t.Fatalf("grant_type=%q", gotForm.Get("grant_type"))
	}
	if gotForm.Get("code") != "auth-code" || gotForm.Get("code_verifier") != "verifier-xyz" {
		t.Fatalf("form=%v", gotForm)
	}
}
