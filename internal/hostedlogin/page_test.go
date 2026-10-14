package hostedlogin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestRenderPasswordFormContainsFieldsAndCSRF(t *testing.T) {
	html := RenderPage(Session{
		ID: "sid1", CSRF: "csrf1", Mode: ModePasswordForm, ConnectorID: "mall",
		PublicOrigin: "http://127.0.0.1:8080",
		Fields: []store.HostedLoginField{
			{Name: "user", Label: "用户名", Arg: "username", Input: "text"},
			{Name: "pass", Label: "密码", Arg: "password", Input: "password"},
		},
	}, "", "zh-CN")
	if !strings.Contains(html, `name="csrf"`) || !strings.Contains(html, "csrf1") {
		t.Fatal(html)
	}
	if !strings.Contains(html, `type="password"`) {
		t.Fatal(html)
	}
	if !strings.Contains(html, "mall") {
		t.Fatal(html)
	}
	if !strings.Contains(html, "data:image/png;base64,") {
		t.Fatal("expected QR data URL")
	}
	if !strings.Contains(html, `lang="zh-CN"`) || !strings.Contains(html, ">登录<") {
		t.Fatalf("want Chinese UI: %s", html)
	}
}

func TestRenderPageEnglish(t *testing.T) {
	html := RenderPage(Session{
		ID: "sid1", CSRF: "csrf1", Mode: ModeTokenPaste, ConnectorID: "mall",
		PublicOrigin: "http://127.0.0.1:8080",
		Fields:       DefaultTokenFields(),
	}, "", "en")
	if !strings.Contains(html, `lang="en"`) {
		t.Fatal(html)
	}
	if !strings.Contains(html, "Sign in") || !strings.Contains(html, "not to the AI model") {
		t.Fatal(html)
	}
	if !strings.Contains(html, `?lang=zh-CN`) || !strings.Contains(html, `?lang=en`) {
		t.Fatal("want language switcher links")
	}
}

func TestRenderPageMethodSwitchWhenMultiple(t *testing.T) {
	html := RenderPage(Session{
		ID: "sid1", CSRF: "csrf1", MethodID: "pwd", Mode: ModeTokenPaste,
		PublicOrigin: "http://127.0.0.1:8080",
		Fields:       DefaultTokenFields(),
		AvailableMethods: []store.HostedLoginMethod{
			{ID: "pwd", Label: "粘贴 Token", Mode: ModeTokenPaste, Fields: DefaultTokenFields()},
			{ID: "basic", Label: "账号密码", Mode: ModeBasic, Fields: DefaultBasicFields()},
		},
	}, "", "zh-CN")
	if !strings.Contains(html, "/v0/auth/hosted/sid1/method") {
		t.Fatal("missing method switch action")
	}
	if !strings.Contains(html, `name="method_id"`) || !strings.Contains(html, `value="basic"`) {
		t.Fatal(html)
	}
	if strings.Contains(html, `value="pwd"`) {
		t.Fatal("current method should not appear as switch target")
	}
	if !strings.Contains(html, "换一种登录方式") {
		t.Fatal(html)
	}
}

func TestRenderPageNoMethodSwitchWhenSingle(t *testing.T) {
	html := RenderPage(Session{
		ID: "sid1", CSRF: "csrf1", MethodID: "pwd", Mode: ModeTokenPaste,
		Fields: DefaultTokenFields(),
		AvailableMethods: []store.HostedLoginMethod{
			{ID: "pwd", Mode: ModeTokenPaste, Fields: DefaultTokenFields()},
		},
	}, "", "en")
	if strings.Contains(html, "/method") {
		t.Fatal("should not render switch for single method")
	}
}

func TestLocaleFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/x?lang=en", nil)
	if got := LocaleFromRequest(req); got != "en" {
		t.Fatalf("got %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/x", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if got := LocaleFromRequest(req); got != "en" {
		t.Fatalf("got %q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/x", nil)
	if got := LocaleFromRequest(req); got != "zh-CN" {
		t.Fatalf("default=%q", got)
	}
}

func TestRenderSuccessPageLocales(t *testing.T) {
	zh := RenderSuccessPage("zh-CN")
	if !strings.Contains(zh, "已登录") || !strings.Contains(zh, `lang="zh-CN"`) {
		t.Fatal(zh)
	}
	en := RenderSuccessPage("en")
	if !strings.Contains(en, "Signed in") || !strings.Contains(en, `lang="en"`) {
		t.Fatal(en)
	}
}
