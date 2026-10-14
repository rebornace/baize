package mcpexport

import (
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/tool"
)

// HostedLoginRequiredCode is the structured tool-result code when browser sign-in is required.
const HostedLoginRequiredCode = "hosted_login_required"

var authFailureCodes = map[string]struct{}{
	"login_required":        {},
	"oauth_reauth_required": {},
}

var authFailureErrSubstrings = []string{
	"401",
	"403",
	"unauthorized",
	"forbidden",
}

// AuthExpiredMessage returns bilingual (zh + en) MCP export auth expiry text.
func AuthExpiredMessage(identityName string) string {
	return AuthExpiredMessageWithURL(identityName, "")
}

// AuthExpiredMessageWithURL returns bilingual expiry instructions. When hostedURL
// is set, the user is directed to the public hosted login page.
func AuthExpiredMessageWithURL(identityName, hostedURL string) string {
	zh, en := authExpiredParts(identityName, hostedURL)
	return zh + "\n\n" + en
}

// AuthExpiredMessageZH / AuthExpiredMessageEN return a single-locale copy.
func AuthExpiredMessageZH(identityName, hostedURL string) string {
	zh, _ := authExpiredParts(identityName, hostedURL)
	return zh
}

func AuthExpiredMessageEN(identityName, hostedURL string) string {
	_, en := authExpiredParts(identityName, hostedURL)
	return en
}

func authExpiredParts(identityName, hostedURL string) (zh, en string) {
	name := strings.TrimSpace(identityName)
	nameZH := name
	nameEN := name
	if name == "" {
		nameZH = "导出身份"
		nameEN = "export identity"
	}
	hostedURL = strings.TrimSpace(hostedURL)
	if hostedURL != "" {
		// Lead with the raw URL so agents that only skim text Content still see it.
		zh = fmt.Sprintf(
			"【中文】需要登录（%s）。请立刻在浏览器打开下面的链接完成登录（切勿在对话中粘贴密码），用户确认后再重试工具：\n%s\n（structured 字段 hosted_login_url 相同）",
			nameZH, hostedURL,
		)
		en = fmt.Sprintf(
			"[English] Sign-in required (%s). Open this URL in a browser now (never paste passwords into chat), then retry the tool after the user confirms:\n%s\n(same value in structured field hosted_login_url)",
			nameEN, hostedURL,
		)
		return zh, en
	}
	zh = fmt.Sprintf(
		"【中文】%s 的会话已过期。请勿重试本工具或其他白泽导出工具。请用户在白泽中重新登录后再继续。",
		nameZH,
	)
	en = fmt.Sprintf(
		"[English] Session for %s expired. Do not retry this or other Baize tools. Ask the user to re-login in Baize, then continue.",
		nameEN,
	)
	return zh, en
}

// HostedLoginElicitMessage is the bilingual URL-elicitation prompt for MCP clients.
func HostedLoginElicitMessage(identityName string) string {
	name := strings.TrimSpace(identityName)
	nameZH := name
	nameEN := name
	if name == "" {
		nameZH = "该系统"
		nameEN = "this system"
	}
	return fmt.Sprintf(
		"【中文】请登录以继续使用白泽导出工具（%s）。请在浏览器打开链接；切勿在对话中粘贴密码。\n\n[English] Sign in to continue using Baize export tools for %s. Open the URL in a browser; never paste passwords into chat.",
		nameZH, nameEN,
	)
}

// IsAuthFailure reports whether a tool invoke result indicates expired or missing auth.
func IsAuthFailure(content map[string]any, isError bool, err error) bool {
	if err != nil {
		msg := strings.ToLower(err.Error())
		for _, sub := range authFailureErrSubstrings {
			if strings.Contains(msg, sub) {
				return true
			}
		}
	}
	if !isError || content == nil {
		return false
	}
	code, _ := content["code"].(string)
	code = strings.TrimSpace(strings.ToLower(code))
	if _, ok := authFailureCodes[code]; ok {
		return true
	}
	// Raw downstream 401/403 payloads (http_status / Chinese expiry copy) must
	// also trigger hosted reauth for MCP clients that never set require_login.
	return tool.LooksLikeAuthFailure(content)
}
