package identity

import (
	"encoding/base64"
	"encoding/json"
	"path"
	"strings"
)

// CaptureConfig describes how to extract credentials from login tool results.
type CaptureConfig struct {
	ToolNameGlob   string
	TokenJSONPaths []string
	LabelJSONPaths []string
	HeaderTemplate string
	DefaultScheme  string
}

// MatchToolName reports whether toolName matches the configured glob pattern.
// Matching is case-insensitive so default "*login*" also covers OpenAPI-style
// names like AuthController_phoneLogin (PascalCase "Login").
func MatchToolName(glob, toolName string) bool {
	if glob == "" {
		return false
	}
	ok, err := path.Match(strings.ToLower(glob), strings.ToLower(toolName))
	return err == nil && ok
}

// ExtractCredential parses token and label from a tool result map.
// Returns headers (e.g. Authorization), label, subject, claims summary, and ok.
func ExtractCredential(cfg CaptureConfig, result map[string]any) (headers map[string]string, label, subject string, claims map[string]any, ok bool) {
	token := firstTokenAtPaths(result, cfg.TokenJSONPaths)
	if token == "" {
		token = findTokenInResult(result, 0)
	}
	if token == "" {
		return nil, "", "", nil, false
	}

	label = firstStringAtPaths(result, cfg.LabelJSONPaths)
	if label == "" {
		label = firstStringAtPaths(result, []string{"email", "data.email", "data.user.email", "user.email", "username", "data.username"})
	}
	claims = ParseJWTClaimsSummary(token)

	subject = label
	if sub, _ := claims["sub"].(string); sub != "" {
		subject = sub
	}

	authValue := buildAuthHeader(cfg.HeaderTemplate, token)
	if authValue == "" {
		return nil, "", "", nil, false
	}

	return map[string]string{"Authorization": authValue}, label, subject, claims, true
}

func buildAuthHeader(template, token string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(token)), "bearer ") {
		return strings.TrimSpace(token)
	}
	if template == "" {
		return "Bearer " + token
	}
	return strings.ReplaceAll(template, "{{token}}", token)
}

func firstStringAtPaths(data map[string]any, paths []string) string {
	for _, p := range paths {
		if v, ok := stringAtPath(data, p); ok && v != "" {
			return v
		}
	}
	return ""
}

func firstTokenAtPaths(data map[string]any, paths []string) string {
	for _, p := range paths {
		v := valueAtPath(data, p)
		if s := coerceOrNestedToken(v, 0); s != "" {
			return s
		}
	}
	return ""
}

func stringAtPath(data map[string]any, path string) (string, bool) {
	v := valueAtPath(data, path)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

var preferredTokenKeys = []string{
	"accessToken", "access_token", "access-token",
	"tokenValue", "token_value",
	"jwt", "idToken", "id_token",
	"token", "authorization", "Authorization",
}

var tokenWrapperKeys = []string{"data", "result", "payload", "content", "body"}

func findTokenInResult(data map[string]any, depth int) string {
	if data == nil || depth > 8 {
		return ""
	}
	for _, k := range preferredTokenKeys {
		if s := coerceOrNestedToken(data[k], depth+1); s != "" {
			return s
		}
	}
	for _, k := range tokenWrapperKeys {
		switch v := data[k].(type) {
		case string:
			if s, ok := coerceTokenString(v); ok {
				return s
			}
		case map[string]any:
			if s := findTokenInResult(v, depth+1); s != "" {
				return s
			}
		}
	}
	return ""
}

func coerceOrNestedToken(v any, depth int) string {
	if s, ok := coerceTokenString(v); ok {
		return s
	}
	if m, ok := v.(map[string]any); ok {
		return findTokenInResult(m, depth)
	}
	return ""
}

func coerceTokenString(v any) (string, bool) {
	s, ok := v.(string)
	if !ok {
		return "", false
	}
	s = strings.TrimSpace(s)
	if !looksLikeToken(s) {
		return "", false
	}
	return s, true
}

func looksLikeToken(s string) bool {
	if len(s) < 8 {
		return false
	}
	if strings.Contains(s, "@") {
		return false
	}
	lower := strings.ToLower(s)
	if strings.Contains(s, " ") && !strings.HasPrefix(lower, "bearer ") {
		return false
	}
	return true
}

func valueAtPath(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	var cur any = data
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil
		}
		cur, ok = m[part]
		if !ok {
			return nil
		}
	}
	return cur
}

// ParseJWTClaimsSummary decodes the JWT payload without verifying the signature.
// Returns an empty map on failure.
func ParseJWTClaimsSummary(token string) map[string]any {
	raw := strings.TrimSpace(token)
	if strings.HasPrefix(strings.ToLower(raw), "bearer ") {
		raw = strings.TrimSpace(raw[7:])
	}

	parts := strings.Split(raw, ".")
	if len(parts) != 3 {
		return map[string]any{}
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return map[string]any{}
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return map[string]any{}
	}
	return claims
}
