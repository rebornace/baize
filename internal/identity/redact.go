package identity

import "strings"

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

// RedactSensitive recursively replaces sensitive map values with "[redacted]".
// Non-map / non-slice values are returned unchanged. The input is not mutated.
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
	default:
		return v
	}
}
