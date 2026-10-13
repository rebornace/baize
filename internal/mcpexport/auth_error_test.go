package mcpexport_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
)

func TestAuthExpiredMessage(t *testing.T) {
	msg := mcpexport.AuthExpiredMessage("Ops")
	if !strings.Contains(msg, "Session for Ops expired") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "Do not retry this or other Baize tools") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "re-login in Baize") {
		t.Fatalf("msg=%q", msg)
	}
}

func TestAuthExpiredMessageEmptyName(t *testing.T) {
	msg := mcpexport.AuthExpiredMessage("")
	if !strings.Contains(msg, "Session for export identity expired") {
		t.Fatalf("msg=%q", msg)
	}
}

func TestAuthExpiredMessageWithURL(t *testing.T) {
	msg := mcpexport.AuthExpiredMessageWithURL("Ops", "http://127.0.0.1:8080/v0/auth/hosted/abc")
	if !strings.Contains(msg, "Session for Ops expired") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "/v0/auth/hosted/abc") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "do not paste passwords") {
		t.Fatalf("msg=%q", msg)
	}
}

func TestClassifyAuthFailure(t *testing.T) {
	if !mcpexport.IsAuthFailure(map[string]any{"code": "login_required"}, true, nil) {
		t.Fatal("login_required")
	}
	if !mcpexport.IsAuthFailure(map[string]any{"code": "oauth_reauth_required"}, true, nil) {
		t.Fatal("oauth_reauth")
	}
	if !mcpexport.IsAuthFailure(nil, false, errors.New("http 401 unauthorized")) {
		t.Fatal("401 err")
	}
	if mcpexport.IsAuthFailure(map[string]any{"ok": true}, false, nil) {
		t.Fatal("false positive")
	}
}
