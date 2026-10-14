package mcpexport_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/mcpexport"
)

func TestAuthExpiredMessageBilingual(t *testing.T) {
	msg := mcpexport.AuthExpiredMessage("Ops")
	if !strings.Contains(msg, "会话已过期") {
		t.Fatalf("missing zh: %q", msg)
	}
	if !strings.Contains(msg, "Session for Ops expired") {
		t.Fatalf("missing en: %q", msg)
	}
	if !strings.Contains(msg, "请勿重试") || !strings.Contains(msg, "Do not retry") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "重新登录") || !strings.Contains(msg, "re-login in Baize") {
		t.Fatalf("msg=%q", msg)
	}
}

func TestAuthExpiredMessageEmptyName(t *testing.T) {
	msg := mcpexport.AuthExpiredMessage("")
	if !strings.Contains(msg, "导出身份") || !strings.Contains(msg, "export identity") {
		t.Fatalf("msg=%q", msg)
	}
}

func TestAuthExpiredMessageWithURL(t *testing.T) {
	msg := mcpexport.AuthExpiredMessageWithURL("Ops", "http://127.0.0.1:8080/v0/auth/hosted/abc")
	if !strings.Contains(msg, "浏览器打开") {
		t.Fatalf("missing zh: %q", msg)
	}
	if !strings.Contains(msg, "/v0/auth/hosted/abc") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "hosted_login_url") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "never paste passwords") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "切勿在对话中粘贴密码") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "Sign-in required") && !strings.Contains(msg, "需要登录") {
		t.Fatalf("msg=%q", msg)
	}
	zh := mcpexport.AuthExpiredMessageZH("Ops", "http://x/v0/auth/hosted/abc")
	en := mcpexport.AuthExpiredMessageEN("Ops", "http://x/v0/auth/hosted/abc")
	if strings.Contains(zh, "[English]") || strings.Contains(en, "【中文】") {
		t.Fatalf("zh=%q en=%q", zh, en)
	}
}

func TestHostedLoginElicitMessage(t *testing.T) {
	msg := mcpexport.HostedLoginElicitMessage("Ops")
	if !strings.Contains(msg, "Ops") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(msg, "请登录") || !strings.Contains(msg, "Sign in") {
		t.Fatalf("msg=%q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "never paste passwords") {
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
	if !mcpexport.IsAuthFailure(map[string]any{"http_status": 401, "message": "Unauthorized"}, true, nil) {
		t.Fatal("http_status 401")
	}
	if !mcpexport.IsAuthFailure(map[string]any{"message": "暂未登录或token已经过期"}, true, nil) {
		t.Fatal("chinese expiry")
	}
	if mcpexport.IsAuthFailure(map[string]any{"ok": true}, false, nil) {
		t.Fatal("false positive")
	}
}
