package hitllink

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func pageFixture() (*store.HITLPayload, Claims) {
	p := &store.HITLPayload{
		ToolName: "create_ticket",
		Prompt:   "确认开单？",
		Kind:     store.HITLKindApproval,
	}
	c := Claims{
		RunID:        "run_1",
		WaitUnixNano: time.Now().UnixNano(),
		ExpiresAt:    time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC),
	}
	return p, c
}

func TestRenderApprovalPage(t *testing.T) {
	p, c := pageFixture()
	html := RenderPage(p, c, "zh-CN", "")
	for _, needle := range []string{
		`lang="zh-CN"`,
		"create_ticket",
		"确认开单？",
		"run_1",
		`method="post"`,
		`name="decision"`,
		`value="approve"`,
		`value="reject"`,
		`name="comment"`,
		"2026-11-21T12:00:00Z",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("missing %q in page:\n%s", needle, html)
		}
	}
}

func TestRenderApprovalPageEscapesUserContent(t *testing.T) {
	p, c := pageFixture()
	p.ToolName = `<script>alert(1)</script>`
	p.Prompt = `<img src=x onerror=alert(2)>`
	html := RenderPage(p, c, "zh-CN", "")
	if strings.Contains(html, "<script>") || strings.Contains(html, "<img") {
		t.Fatalf("raw user HTML leaked:\n%s", html)
	}
	if !strings.Contains(html, "&lt;script&gt;") || !strings.Contains(html, "&lt;img") {
		t.Fatalf("expected escaped content:\n%s", html)
	}
}

func TestRenderPageHasNoExternalResources(t *testing.T) {
	p, c := pageFixture()
	html := RenderPage(p, c, "zh-CN", "")
	lower := strings.ToLower(html)
	if strings.Contains(lower, "http://") || strings.Contains(lower, "https://") {
		t.Fatalf("page must not load external resources:\n%s", html)
	}
	if strings.Contains(lower, "<script") || strings.Contains(lower, "<link") {
		t.Fatalf("page must not include scripts or external styles:\n%s", html)
	}
}

func TestLocaleFromRequest(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/v0/hitl/approvals/x?lang=en", nil)
	if got := LocaleFromRequest(req); got != "en" {
		t.Fatalf("query locale=%q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/hitl/approvals/x", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if got := LocaleFromRequest(req); got != "en" {
		t.Fatalf("header locale=%q", got)
	}
	req = httptest.NewRequest(http.MethodGet, "/v0/hitl/approvals/x", nil)
	if got := LocaleFromRequest(req); got != "zh-CN" {
		t.Fatalf("default locale=%q", got)
	}
}

func TestRenderEnglishPage(t *testing.T) {
	p, c := pageFixture()
	html := RenderPage(p, c, "en", "")
	if !strings.Contains(html, `lang="en"`) || !strings.Contains(html, "Approval required") {
		t.Fatalf("english page missing:\n%s", html)
	}
	if !strings.Contains(html, "Approve") || !strings.Contains(html, "Reject") {
		t.Fatalf("english actions missing:\n%s", html)
	}
}

func TestRenderMessagePageEscapes(t *testing.T) {
	html := RenderMessagePage("en", `<b>Handled</b>`, `<script>x</script>`)
	if strings.Contains(html, "<b>Handled</b>") || strings.Contains(html, "<script>") {
		t.Fatalf("message not escaped:\n%s", html)
	}
	if !strings.Contains(html, "&lt;b&gt;Handled&lt;/b&gt;") {
		t.Fatalf("expected escaped title:\n%s", html)
	}
}
