package tool

import (
	"fmt"
	"strconv"
	"strings"
)

// LoginSignals is per-connector auth-failure recognition. Empty means built-in
// 401/403 heuristics (see MatchAuthFailure).
type LoginSignals struct {
	HTTPStatuses           []int
	HTTPStatusesNeedSignal []int
	BodyCodePath           string
	BodyCodes              []string
	MessageContains        []string
}

// Empty reports whether no login-signal fields are set.
func (s LoginSignals) Empty() bool {
	return len(s.HTTPStatuses) == 0 &&
		len(s.HTTPStatusesNeedSignal) == 0 &&
		strings.TrimSpace(s.BodyCodePath) == "" &&
		len(s.BodyCodes) == 0 &&
		len(s.MessageContains) == 0
}

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

// LooksLikeAuthFailure reports 401/403-style payloads using built-in heuristics.
func LooksLikeAuthFailure(content map[string]any) bool {
	return MatchAuthFailure(content, LoginSignals{})
}

// ambiguousHTTPStatuses are often ordinary business errors (bad params / missing
// resource). Even if an operator lists them under HTTPStatuses (alone), treat
// them as NeedSignal so a logged-in session is not remapped to login_required
// on every 400/404.
var ambiguousHTTPStatuses = map[int]struct{}{
	400: {},
	404: {},
}

// NormalizeLoginSignals moves ambiguous statuses out of the "alone" list into
// NeedSignal so misconfigured connectors (e.g. http_statuses: 400,401,403,404)
// do not treat ordinary business errors as auth failures.
func NormalizeLoginSignals(signals LoginSignals) LoginSignals {
	if signals.Empty() {
		return signals
	}
	alone := make([]int, 0, len(signals.HTTPStatuses))
	need := append([]int(nil), signals.HTTPStatusesNeedSignal...)
	seenNeed := map[int]struct{}{}
	for _, s := range need {
		seenNeed[s] = struct{}{}
	}
	for _, s := range signals.HTTPStatuses {
		if _, amb := ambiguousHTTPStatuses[s]; amb {
			if _, ok := seenNeed[s]; !ok {
				need = append(need, s)
				seenNeed[s] = struct{}{}
			}
			continue
		}
		alone = append(alone, s)
	}
	signals.HTTPStatuses = alone
	signals.HTTPStatusesNeedSignal = need
	return signals
}

// MatchAuthFailure reports whether content should be treated as an auth failure.
// Empty signals use built-in 401/403 / message heuristics. When any field is set,
// that dimension uses the configured values and unset dimensions keep defaults
// (statuses 401/403; body codes 401/403/unauthorized/…; built-in message phrases).
// HTTPStatusesNeedSignal has no built-in list (empty means none).
func MatchAuthFailure(content map[string]any, signals LoginSignals) bool {
	if content == nil {
		return false
	}
	if IsLoginRequired(content) {
		return true
	}
	if signals.Empty() {
		return builtinAuthFailure(content)
	}
	signals = NormalizeLoginSignals(signals)

	msgHit := messageSignalHit(content, signals.MessageContains, len(signals.MessageContains) == 0)
	bodyPath := strings.TrimSpace(signals.BodyCodePath)
	if bodyPath == "" {
		bodyPath = "code"
	}
	bodyHit := bodyCodeSignalHit(content, bodyPath, signals.BodyCodes, len(signals.BodyCodes) == 0)
	status := contentHTTPStatus(content)

	if msgHit {
		return true
	}
	statuses := signals.HTTPStatuses
	if len(statuses) == 0 {
		statuses = []int{401, 403}
	}
	for _, s := range statuses {
		if status == s {
			return true
		}
	}
	for _, s := range signals.HTTPStatusesNeedSignal {
		if status == s && (bodyHit || msgHit) {
			return true
		}
	}
	// HTTP 200 (or missing status) with a business auth code in the body.
	if bodyHit && (status == 0 || (status >= 200 && status < 300)) {
		return true
	}
	return false
}

func builtinAuthFailure(content map[string]any) bool {
	status := contentHTTPStatus(content)
	if status == 401 || status == 403 {
		return true
	}
	if bodyCodeSignalHit(content, "code", nil, true) {
		return true
	}
	return messageSignalHit(content, nil, true)
}

func contentHTTPStatus(content map[string]any) int {
	switch v := content["http_status"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

func messageSignalHit(content map[string]any, needles []string, useDefault bool) bool {
	texts := []string{}
	if msg, ok := content["message"].(string); ok {
		texts = append(texts, msg)
	}
	if errMsg, ok := content["error"].(string); ok {
		texts = append(texts, errMsg)
	}
	if len(texts) == 0 {
		return false
	}
	if useDefault {
		for _, text := range texts {
			m := strings.ToLower(strings.TrimSpace(text))
			if m == "unauthorized" || m == "forbidden" ||
				strings.Contains(m, "unauthorized") ||
				strings.Contains(m, "authentication is required") ||
				strings.Contains(m, "未登录") ||
				strings.Contains(m, "token已经过期") ||
				strings.Contains(m, "token已过期") ||
				strings.Contains(m, "登录已过期") {
				return true
			}
			if strings.Contains(m, "401") {
				return true
			}
		}
		return false
	}
	for _, needle := range needles {
		n := strings.TrimSpace(needle)
		if n == "" {
			continue
		}
		lower := strings.ToLower(n)
		for _, text := range texts {
			if strings.Contains(strings.ToLower(text), lower) {
				return true
			}
		}
	}
	return false
}

func bodyCodeSignalHit(content map[string]any, path string, codes []string, useDefault bool) bool {
	raw, ok := valueAtPath(content, path)
	if !ok {
		return false
	}
	got := stringifyCode(raw)
	if got == "" {
		return false
	}
	if useDefault {
		c := strings.TrimSpace(got)
		if c == "401" || c == "403" || c == "login_required" ||
			strings.EqualFold(c, "unauthorized") || strings.EqualFold(c, "forbidden") {
			return true
		}
		return false
	}
	for _, want := range codes {
		w := strings.TrimSpace(want)
		if w == "" {
			continue
		}
		if strings.EqualFold(got, w) {
			return true
		}
	}
	return false
}

func valueAtPath(content map[string]any, path string) (any, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, false
	}
	cur := any(content)
	for _, part := range strings.Split(path, ".") {
		part = strings.TrimSpace(part)
		if part == "" {
			return nil, false
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

func stringifyCode(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case int:
		return strconv.Itoa(t)
	case int64:
		return strconv.FormatInt(t, 10)
	case bool:
		return strconv.FormatBool(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// NeedsLoginPause reports whether the engine should stop and wait for login / skip.
func NeedsLoginPause(content map[string]any) bool {
	return IsLoginRequired(content) || LooksLikeAuthFailure(content)
}

// NormalizeLoginRequired rewrites auth-failure payloads to login_required for UI/engine.
func NormalizeLoginRequired(content map[string]any) map[string]any {
	return NormalizeLoginRequiredWith(content, LoginSignals{})
}

// NormalizeLoginRequiredWith is like NormalizeLoginRequired with per-connector signals.
func NormalizeLoginRequiredWith(content map[string]any, signals LoginSignals) map[string]any {
	if IsLoginRequired(content) {
		return content
	}
	if !MatchAuthFailure(content, signals) {
		return content
	}
	msg := "登录已失效或未登录，请点击「浏览器登录」、「忽略当前步骤」或「忽略此系统」"
	if m, ok := content["message"].(string); ok {
		if t := strings.TrimSpace(m); t != "" {
			msg = t + " — 请点击「浏览器登录」、「忽略当前步骤」或「忽略此系统」"
		}
	}
	out := LoginRequiredContentMsg(msg)
	if cid, ok := content["connector_id"].(string); ok {
		if t := strings.TrimSpace(cid); t != "" {
			out["connector_id"] = t
		}
	}
	// Keep enough of the original error for operators to see why it remapped.
	if st := contentHTTPStatus(content); st > 0 {
		out["http_status"] = st
	}
	if raw, ok := valueAtPath(content, "code"); ok {
		if s := stringifyCode(raw); s != "" && s != "login_required" {
			out["upstream_code"] = raw
		}
	}
	return out
}

// RemapAuthFailureContent rewrites a tool error to login_required when signals match.
func RemapAuthFailureContent(content map[string]any, signals LoginSignals, connectorID string) (map[string]any, bool) {
	if content == nil || IsLoginRequired(content) {
		return content, IsLoginRequired(content)
	}
	if !MatchAuthFailure(content, signals) {
		return content, false
	}
	out := NormalizeLoginRequiredWith(content, signals)
	if cid := strings.TrimSpace(connectorID); cid != "" {
		if _, has := out["connector_id"]; !has {
			out["connector_id"] = cid
		}
	}
	return out, true
}
