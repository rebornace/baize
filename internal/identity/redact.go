package identity

import (
	"regexp"
	"strings"
)

// sensitiveKeys are JSON object keys whose values must not appear in stored events / public views.
var sensitiveKeys = map[string]struct{}{
	"accesstoken":   {},
	"access_token":  {},
	"refreshtoken":  {},
	"refresh_token": {},
	"idtoken":       {},
	"id_token":      {},
	"token":         {},
	"password":      {},
	"secret":        {},
	"authorization": {},
}

// textSecretPatterns cover common key/value and bearer shapes inside strings.
// Order matters: bearer first, otherwise "Authorization: Bearer x" would leave
// the token behind.
var textSecretPatterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	{
		re:   regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._~+/\-]{8,}=*`),
		repl: `Bearer ***`,
	},
	{
		re:   regexp.MustCompile(`(?i)\b(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|token|secret|password|passwd)\b\s*[:=]\s*([^\s,;]+)`),
		repl: `${1}: ***`,
	},
	{
		re:   regexp.MustCompile(`\b(sk|pk)-[A-Za-z0-9]{16,}`),
		repl: `***`,
	},
}

// RedactText masks common credential shapes inside free-form text.
func RedactText(s string) string {
	out := strings.TrimSpace(s)
	for _, p := range textSecretPatterns {
		out = p.re.ReplaceAllString(out, p.repl)
	}
	return out
}

// RedactSensitive recursively replaces sensitive map values with "[redacted]"
// and masks credential shapes inside strings. Non-map / non-slice values are
// otherwise returned unchanged. The input is not mutated.
func RedactSensitive(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			if _, ok := sensitiveKeys[strings.ToLower(k)]; ok {
				out[k] = "[redacted]"
			} else {
				out[k] = RedactSensitive(val)
			}
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = RedactSensitive(val)
		}
		return out
	case string:
		return RedactText(x)
	default:
		return v
	}
}
