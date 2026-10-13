package hostedlogin

import (
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestRenderPasswordFormContainsFieldsAndCSRF(t *testing.T) {
	html := RenderPage(Session{
		CSRF: "csrf1", Mode: ModePasswordForm, ConnectorID: "mall",
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
}
