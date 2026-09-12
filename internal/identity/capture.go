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
func MatchToolName(glob, toolName string) bool {
	if glob == "" {
		return false
	}
	ok, err := path.Match(glob, toolName)
	return err == nil && ok
}

// ExtractCredential parses token and label from a tool result map.
// Returns headers (e.g. Authorization), label, subject, claims summary, and ok.
func ExtractCredential(cfg CaptureConfig, result map[string]any) (headers map[string]string, label, subject string, claims map[string]any, ok bool) {
	token := firstStringAtPaths(result, cfg.TokenJSONPaths)
	if token == "" {
		return nil, "", "", nil, false
	}

	label = firstStringAtPaths(result, cfg.LabelJSONPaths)
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

func stringAtPath(data map[string]any, path string) (string, bool) {
	v := valueAtPath(data, path)
	if v == nil {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
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
