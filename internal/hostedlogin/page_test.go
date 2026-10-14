package hostedlogin

import (
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
	}, "")
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
	}, "")
	if !strings.Contains(html, "/v0/auth/hosted/sid1/method") {
		t.Fatal("missing method switch action")
	}
	if !strings.Contains(html, `name="method_id"`) || !strings.Contains(html, `value="basic"`) {
		t.Fatal(html)
	}
	if strings.Contains(html, `value="pwd"`) {
		t.Fatal("current method should not appear as switch target")
	}
}

func TestRenderPageNoMethodSwitchWhenSingle(t *testing.T) {
	html := RenderPage(Session{
		ID: "sid1", CSRF: "csrf1", MethodID: "pwd", Mode: ModeTokenPaste,
		Fields: DefaultTokenFields(),
		AvailableMethods: []store.HostedLoginMethod{
			{ID: "pwd", Mode: ModeTokenPaste, Fields: DefaultTokenFields()},
		},
	}, "")
	if strings.Contains(html, "/method") {
		t.Fatal("should not render switch for single method")
	}
}
