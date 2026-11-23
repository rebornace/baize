package hitllink

import (
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/store"
)

type pageCopy struct {
	lang    string
	title   string
	heading string
	tool    string
	prompt  string
	run     string
	expires string
	comment string
	approve string
	reject  string
	footer  string
	notice  string
}

func copyFor(locale string) pageCopy {
	if strings.HasPrefix(strings.ToLower(locale), "en") {
		return pageCopy{
			lang:    "en",
			title:   "Approval required",
			heading: "Approval required",
			tool:    "Tool",
			prompt:  "Request",
			run:     "Run",
			expires: "Expires",
			comment: "Comment (optional)",
			approve: "Approve and continue",
			reject:  "Reject",
			footer:  "This link is valid only for this wait and stops working once the approval is handled.",
			notice:  "Heads up",
		}
	}
	return pageCopy{
		lang:    "zh-CN",
		title:   "审批请求",
		heading: "需要你确认",
		tool:    "工具",
		prompt:  "说明",
		run:     "运行",
		expires: "到期",
		comment: "备注（可选）",
		approve: "同意并继续",
		reject:  "拒绝",
		footer:  "此链接只对本次等待有效；审批处理完成后自动失效。",
		notice:  "提示",
	}
}

// LocaleFromRequest resolves the small locale set used by the approval page.
func LocaleFromRequest(r *http.Request) string {
	if r != nil {
		if lang := strings.TrimSpace(r.URL.Query().Get("lang")); lang != "" {
			if strings.HasPrefix(strings.ToLower(lang), "en") {
				return "en"
			}
			return "zh-CN"
		}
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Accept-Language")), "en") {
			return "en"
		}
	}
	return "zh-CN"
}

// RenderPage renders the mobile approval form. User-controlled fields are
// escaped; the form posts back to the same token URL.
func RenderPage(p *store.HITLPayload, claims Claims, locale, notice string) string {
	c := copyFor(locale)
	tool := ""
	prompt := ""
	if p != nil {
		tool = strings.TrimSpace(p.ToolName)
		prompt = strings.TrimSpace(p.Prompt)
	}
	expires := claims.ExpiresAt.UTC().Format(time.RFC3339)
	if claims.ExpiresAt.IsZero() {
		expires = ""
	}
	noticeHTML := ""
	if strings.TrimSpace(notice) != "" {
		noticeHTML = `<p class="notice"><strong>` + html.EscapeString(c.notice) + `:</strong> ` +
			html.EscapeString(strings.TrimSpace(notice)) + `</p>`
	}
	return `<!DOCTYPE html>
<html lang="` + c.lang + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(c.title) + `</title>
<style>
:root{color-scheme:light dark}
*{box-sizing:border-box}
body{margin:0;padding:24px 16px 40px;font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#f5f4ef;color:#1f2933}
main{max-width:34rem;margin:0 auto}
.card{background:#fff;border:1px solid #e4e1d8;border-radius:16px;padding:20px;box-shadow:0 8px 24px rgba(15,61,62,.08)}
h1{font-size:1.4rem;line-height:1.25;margin:0 0 16px;color:#0f3d3e}
dl{margin:0 0 18px}
dt{font-size:.75rem;letter-spacing:.04em;text-transform:uppercase;color:#6b7280;margin-top:14px}
dt:first-child{margin-top:0}
dd{margin:4px 0 0;font-size:1rem;line-height:1.5;white-space:pre-wrap;overflow-wrap:anywhere}
textarea{width:100%;min-height:84px;margin-top:16px;padding:12px;border:1px solid #d7d3c8;border-radius:10px;font:inherit;background:#fbfaf7;color:inherit}
.actions{display:grid;grid-template-columns:1fr 1fr;gap:10px;margin-top:18px}
button{min-height:48px;border:0;border-radius:10px;font:inherit;font-weight:650;cursor:pointer}
.approve{background:#0f6b62;color:#fff}
.reject{background:#f0ece2;color:#5b3a2e}
.notice{padding:12px;border-radius:10px;background:#fff4d6;color:#6b4e00;font-size:.9rem}
.footer{max-width:34rem;margin:16px auto 0;padding:0 4px;color:#6b7280;font-size:.8rem;line-height:1.5}
@media (max-width:360px){.actions{grid-template-columns:1fr}}
</style>
</head>
<body>
<main>
<section class="card">
<h1>` + html.EscapeString(c.heading) + `</h1>
` + noticeHTML + `
<dl>
<dt>` + html.EscapeString(c.tool) + `</dt><dd>` + html.EscapeString(tool) + `</dd>
<dt>` + html.EscapeString(c.prompt) + `</dt><dd>` + html.EscapeString(prompt) + `</dd>
<dt>` + html.EscapeString(c.run) + `</dt><dd>` + html.EscapeString(claims.RunID) + `</dd>
<dt>` + html.EscapeString(c.expires) + `</dt><dd>` + html.EscapeString(expires) + `</dd>
</dl>
<form method="post" action="">
<textarea name="comment" maxlength="2000" placeholder="` + html.EscapeString(c.comment) + `"></textarea>
<div class="actions">
<button class="approve" type="submit" name="decision" value="approve">` + html.EscapeString(c.approve) + `</button>
<button class="reject" type="submit" name="decision" value="reject">` + html.EscapeString(c.reject) + `</button>
</div>
</form>
</section>
<p class="footer">` + html.EscapeString(c.footer) + `</p>
</main>
</body>
</html>`
}

// RenderMessagePage renders a neutral result/error page. Title and body are
// escaped by the renderer.
func RenderMessagePage(locale, title, body string) string {
	c := copyFor(locale)
	return `<!DOCTYPE html>
<html lang="` + c.lang + `">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>` + html.EscapeString(title) + `</title>
<style>
:root{color-scheme:light dark}
body{margin:0;padding:40px 20px;font-family:system-ui,-apple-system,"Segoe UI",sans-serif;background:#f5f4ef;color:#1f2933}
main{max-width:32rem;margin:0 auto;background:#fff;border:1px solid #e4e1d8;border-radius:16px;padding:24px;line-height:1.6}
h1{font-size:1.35rem;margin:0 0 12px;color:#0f3d3e}
p{margin:0;white-space:pre-wrap;overflow-wrap:anywhere}
</style>
</head>
<body><main><h1>` + html.EscapeString(title) + `</h1><p>` + html.EscapeString(body) + `</p></main></body>
</html>`
}
