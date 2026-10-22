package tool

import "strings"

// LoginRequiredContent is the tool result when invoke is blocked for missing login.
func LoginRequiredContent() map[string]any {
	return LoginRequiredContentMsg("此工具需要先登录")
}

// LoginRequiredContentMsg is like LoginRequiredContent with a custom message
// (e.g. downstream 401 / expired token). Keep code=login_required so Chat ToolCard
// shows「浏览器登录」.
func LoginRequiredContentMsg(message string) map[string]any {
	msg := strings.TrimSpace(message)
	if msg == "" {
		msg = "此工具需要先登录"
	}
	return map[string]any{"code": "login_required", "message": msg}
}

// IsLoginRequired reports whether a tool result is the login gate payload.
func IsLoginRequired(content map[string]any) bool {
	if content == nil {
		return false
	}
	code, _ := content["code"].(string)
	return strings.TrimSpace(code) == "login_required"
}

// LooksLikeAuthFailure reports 401/403-style payloads (including Chinese expiry copy).
func LooksLikeAuthFailure(content map[string]any) bool {
	if content == nil {
		return false
	}
	switch v := content["http_status"].(type) {
	case float64:
		if int(v) == 401 || int(v) == 403 {
			return true
		}
	case int:
		if v == 401 || v == 403 {
			return true
		}
	}
	switch v := content["code"].(type) {
	case float64:
		if int(v) == 401 || int(v) == 403 {
			return true
		}
	case int:
		if v == 401 || v == 403 {
			return true
		}
	case string:
		c := strings.TrimSpace(v)
		if c == "401" || c == "403" || c == "login_required" ||
			strings.EqualFold(c, "unauthorized") || strings.EqualFold(c, "forbidden") {
			return true
		}
	}
	if msg, ok := content["message"].(string); ok {
		m := strings.ToLower(strings.TrimSpace(msg))
		if m == "unauthorized" || m == "forbidden" ||
			strings.Contains(m, "unauthorized") ||
			strings.Contains(m, "authentication is required") ||
			strings.Contains(m, "未登录") ||
			strings.Contains(m, "token已经过期") ||
			strings.Contains(m, "token已过期") ||
			strings.Contains(m, "登录已过期") {
			return true
		}
	}
	if errMsg, ok := content["error"].(string); ok {
		m := strings.ToLower(errMsg)
		if strings.Contains(m, "401") || strings.Contains(m, "unauthorized") ||
			strings.Contains(m, "未登录") {
			return true
		}
	}
	return false
}

// NeedsLoginPause reports whether the engine should stop and wait for login / skip.
func NeedsLoginPause(content map[string]any) bool {
	return IsLoginRequired(content) || LooksLikeAuthFailure(content)
}

// NormalizeLoginRequired rewrites auth-failure payloads to login_required for UI/engine.
func NormalizeLoginRequired(content map[string]any) map[string]any {
	if IsLoginRequired(content) {
		return content
	}
	if !LooksLikeAuthFailure(content) {
		return content
	}
	msg := "登录已失效或未登录，请点击「浏览器登录」、「忽略当前步骤」或「忽略此系统」"
	if m, ok := content["message"].(string); ok {
		if t := strings.TrimSpace(m); t != "" {
			msg = t + " — 请点击「浏览器登录」、「忽略当前步骤」或「忽略此系统」"
		}
	}
	out := LoginRequiredContentMsg(msg)
	// Preserve connector_id so Chat can open hosted login without a fresh catalog.
	if cid, ok := content["connector_id"].(string); ok {
		if t := strings.TrimSpace(cid); t != "" {
			out["connector_id"] = t
		}
	}
	return out
}
