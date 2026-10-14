package hostedlogin

import (
	"encoding/base64"
	"fmt"
	"html"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

type pageCopy struct {
	htmlLang     string
	titlePrefix  string
	metaCreds    string
	qrHint       string
	submit       string
	switchMethod string
	langSwitchZH string
	langSwitchEN string
	successTitle string
	successBody  string
}

func pageCopyFor(locale string) pageCopy {
	if isEnglish(locale) {
		return pageCopy{
			htmlLang:     "en",
			titlePrefix:  "Sign in",
			metaCreds:    "Credentials are sent only to Baize, not to the AI model.",
			qrHint:       "Scan to open this page on your phone:",
			submit:       "Sign in",
			switchMethod: "Use another sign-in method:",
			langSwitchZH: "中文",
			langSwitchEN: "English",
			successTitle: "Signed in",
			successBody:  "You can close this page. If a tool call was waiting, it will continue automatically.",
		}
	}
	return pageCopy{
		htmlLang:     "zh-CN",
		titlePrefix:  "登录",
		metaCreds:    "凭证仅发送到白泽，不会进入 AI 模型。",
		qrHint:       "手机扫码打开本页：",
		submit:       "登录",
		switchMethod: "换一种登录方式：",
		langSwitchZH: "中文",
		langSwitchEN: "English",
		successTitle: "已登录",
		successBody:  "可以关闭本页。若有工具调用正在等待，将自动继续。",
	}
}

// RenderPage returns a minimal HTML login form for the hosted session.
// locale is zh-CN or en (see NormalizeLocale). errMsg is optional.
func RenderPage(sess Session, errMsg, locale string) string {
	locale = NormalizeLocale(locale)
	copy := pageCopyFor(locale)
	title := copy.titlePrefix
	if sess.ConnectorID != "" {
		title = copy.titlePrefix + " — " + sess.ConnectorID
	}
	pageURL := hostedPageURL(sess)
	var b strings.Builder
	fmt.Fprintf(&b, "<!DOCTYPE html>\n<html lang=\"%s\"><head><meta charset=\"utf-8\">", copy.htmlLang)
	b.WriteString("<meta name=\"viewport\" content=\"width=device-width, initial-scale=1\">")
	b.WriteString("<title>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</title>")
	b.WriteString(`<style>
body{font-family:system-ui,sans-serif;max-width:28rem;margin:2rem auto;padding:0 1rem;color:#1c1f26}
label{display:block;margin:.75rem 0 .25rem;font-size:.9rem}
input{width:100%;box-sizing:border-box;padding:.5rem .6rem;border:1px solid #cfd5df;border-radius:8px}
button{margin-top:1rem;padding:.55rem 1rem;border:0;border-radius:8px;background:#2563eb;color:#fff;font-weight:600;cursor:pointer}
.err{color:#dc2626;margin:.75rem 0}
.meta{color:#6b7280;font-size:.85rem}
.qr{display:block;margin:1rem auto;width:160px;height:160px}
.langs{margin:1rem 0;font-size:.85rem}
.langs a{margin-right:.75rem;color:#2563eb}
</style></head><body>`)
	b.WriteString("<h1>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h1>")
	writeLangSwitch(&b, pageURL, locale, copy)
	fmt.Fprintf(&b, `<p class="meta">%s</p>`, html.EscapeString(copy.metaCreds))
	writeMethodSwitch(&b, sess, locale, copy)
	if pageURL != "" {
		if img, err := qrDataURL(pageURL); err == nil {
			fmt.Fprintf(&b, `<p class="meta">%s</p>`, html.EscapeString(copy.qrHint))
			fmt.Fprintf(&b, `<img class="qr" alt="QR" width="160" height="160" src="%s">`, img)
		}
	}
	if strings.TrimSpace(errMsg) != "" {
		b.WriteString(`<p class="err" role="alert">`)
		b.WriteString(html.EscapeString(errMsg))
		b.WriteString("</p>")
	}
	b.WriteString(`<form method="POST" action="">`)
	fmt.Fprintf(&b, `<input type="hidden" name="csrf" value="%s">`, html.EscapeString(sess.CSRF))
	fmt.Fprintf(&b, `<input type="hidden" name="lang" value="%s">`, html.EscapeString(locale))
	fields := sess.Fields
	if len(fields) == 0 {
		switch sess.Mode {
		case ModeTokenPaste:
			fields = DefaultTokenFields()
		case ModeBasic:
			fields = DefaultBasicFields()
		case ModeHeaderPairs:
			fields = DefaultHeaderPairFields()
		}
	}
	for _, f := range fields {
		name := strings.TrimSpace(f.Name)
		if name == "" {
			name = strings.TrimSpace(f.Arg)
		}
		label := strings.TrimSpace(f.Label)
		if label == "" {
			label = name
		}
		input := strings.ToLower(strings.TrimSpace(f.Input))
		if input == "" {
			input = "text"
		}
		b.WriteString("<label for=\"")
		b.WriteString(html.EscapeString(name))
		b.WriteString("\">")
		b.WriteString(html.EscapeString(label))
		b.WriteString("</label>")
		fmt.Fprintf(&b, `<input id="%s" name="%s" type="%s" autocomplete="off" required>`,
			html.EscapeString(name), html.EscapeString(name), html.EscapeString(input))
	}
	fmt.Fprintf(&b, `<button type="submit">%s</button></form></body></html>`, html.EscapeString(copy.submit))
	return b.String()
}

func writeLangSwitch(b *strings.Builder, pageURL, locale string, copy pageCopy) {
	if pageURL == "" {
		return
	}
	b.WriteString(`<p class="langs">`)
	fmt.Fprintf(b, `<a href="%s">%s</a>`, html.EscapeString(pageURL+"?lang=zh-CN"), html.EscapeString(copy.langSwitchZH))
	fmt.Fprintf(b, `<a href="%s">%s</a>`, html.EscapeString(pageURL+"?lang=en"), html.EscapeString(copy.langSwitchEN))
	b.WriteString(`</p>`)
	_ = locale
}

func writeMethodSwitch(b *strings.Builder, sess Session, locale string, copy pageCopy) {
	methods := sess.AvailableMethods
	if len(methods) <= 1 {
		return
	}
	current := strings.TrimSpace(sess.MethodID)
	action := hostedMethodSwitchURL(sess)
	if action == "" {
		return
	}
	fmt.Fprintf(b, `<p class="meta">%s</p>`, html.EscapeString(copy.switchMethod))
	b.WriteString(`<ul class="meta">`)
	for _, m := range methods {
		if m.ID == current {
			continue
		}
		label := strings.TrimSpace(m.Label)
		if label == "" {
			label = m.ID
		}
		b.WriteString(`<li><form method="POST" action="`)
		b.WriteString(html.EscapeString(action))
		b.WriteString(`" style="display:inline">`)
		fmt.Fprintf(b, `<input type="hidden" name="csrf" value="%s">`, html.EscapeString(sess.CSRF))
		fmt.Fprintf(b, `<input type="hidden" name="method_id" value="%s">`, html.EscapeString(m.ID))
		fmt.Fprintf(b, `<input type="hidden" name="lang" value="%s">`, html.EscapeString(NormalizeLocale(locale)))
		b.WriteString(`<button type="submit" style="background:none;border:0;padding:0;color:#2563eb;cursor:pointer;text-decoration:underline;font:inherit">`)
		b.WriteString(html.EscapeString(label))
		b.WriteString(`</button></form></li>`)
	}
	b.WriteString(`</ul>`)
}

func hostedMethodSwitchURL(sess Session) string {
	base := hostedPageURL(sess)
	if base == "" {
		return ""
	}
	return base + "/method"
}

func hostedPageURL(sess Session) string {
	origin := strings.TrimRight(strings.TrimSpace(sess.PublicOrigin), "/")
	id := strings.TrimSpace(sess.ID)
	if origin == "" || id == "" {
		return ""
	}
	return fmt.Sprintf("%s/v0/auth/hosted/%s", origin, id)
}

func qrDataURL(payload string) (string, error) {
	png, err := qrcode.Encode(payload, qrcode.Medium, 160)
	if err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}

// RenderSuccessPage is shown after credentials are captured.
func RenderSuccessPage(locale string) string {
	copy := pageCopyFor(locale)
	return fmt.Sprintf(`<!DOCTYPE html><html lang="%s"><head><meta charset="utf-8"><title>%s</title>
<style>body{font-family:system-ui,sans-serif;max-width:28rem;margin:2rem auto;padding:0 1rem}</style>
</head><body><h1>%s</h1><p>%s</p></body></html>`,
		html.EscapeString(copy.htmlLang),
		html.EscapeString(copy.successTitle),
		html.EscapeString(copy.successTitle),
		html.EscapeString(copy.successBody),
	)
}
