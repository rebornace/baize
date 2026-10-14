package hostedlogin

import (
	"encoding/base64"
	"fmt"
	"html"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

// RenderPage returns a minimal HTML login form for the hosted session.
// errMsg is optional and shown as an alert above the form.
func RenderPage(sess Session, errMsg string) string {
	title := "Sign in"
	if sess.ConnectorID != "" {
		title = "Sign in — " + sess.ConnectorID
	}
	pageURL := hostedPageURL(sess)
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"zh-CN\"><head><meta charset=\"utf-8\">")
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
</style></head><body>`)
	b.WriteString("<h1>")
	b.WriteString(html.EscapeString(title))
	b.WriteString("</h1>")
	b.WriteString(`<p class="meta">Credentials are sent only to Baize, not to the AI model.</p>`)
	writeMethodSwitch(&b, sess)
	if pageURL != "" {
		if img, err := qrDataURL(pageURL); err == nil {
			b.WriteString(`<p class="meta">Scan to open this page on your phone:</p>`)
			fmt.Fprintf(&b, `<img class="qr" alt="Login QR" width="160" height="160" src="%s">`, img)
		}
	}
	if strings.TrimSpace(errMsg) != "" {
		b.WriteString(`<p class="err" role="alert">`)
		b.WriteString(html.EscapeString(errMsg))
		b.WriteString("</p>")
	}
	b.WriteString(`<form method="POST" action="">`)
	fmt.Fprintf(&b, `<input type="hidden" name="csrf" value="%s">`, html.EscapeString(sess.CSRF))
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
	b.WriteString(`<button type="submit">Sign in</button></form></body></html>`)
	return b.String()
}

func writeMethodSwitch(b *strings.Builder, sess Session) {
	methods := sess.AvailableMethods
	if len(methods) <= 1 {
		return
	}
	current := strings.TrimSpace(sess.MethodID)
	action := hostedMethodSwitchURL(sess)
	if action == "" {
		return
	}
	b.WriteString(`<p class="meta">换一种登录方式：</p>`)
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
func RenderSuccessPage() string {
	return `<!DOCTYPE html><html lang="zh-CN"><head><meta charset="utf-8"><title>Signed in</title>
<style>body{font-family:system-ui,sans-serif;max-width:28rem;margin:2rem auto;padding:0 1rem}</style>
</head><body><h1>Signed in</h1><p>You can close this page. If a tool call was waiting, it will continue automatically.</p></body></html>`
}
