package mcp

import (
	"os"
	"regexp"
	"strings"
)

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// ResolveEnv resolves MCP stdio env values and returns KEY=VAL entries for exec.Cmd.
func ResolveEnv(env map[string]string) ([]string, error) {
	resolved, err := resolveKeyValues(env)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(resolved))
	for k, v := range resolved {
		out = append(out, k+"="+v)
	}
	return out, nil
}

// ResolveHeaders resolves MCP HTTP header values (${VAR}, env:, file:).
func ResolveHeaders(headers map[string]string) (map[string]string, error) {
	return resolveKeyValues(headers)
}

func resolveKeyValues(in map[string]string) (map[string]string, error) {
	if len(in) == 0 {
		return map[string]string{}, nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		resolved, err := resolveValue(v)
		if err != nil {
			return nil, err
		}
		out[k] = resolved
	}
	return out, nil
}

func resolveValue(v string) (string, error) {
	switch {
	case strings.HasPrefix(v, "env:"):
		val, err := resolveEnvRef(v)
		if err != nil {
			return "", err
		}
		return val, nil
	case strings.HasPrefix(v, "file:"):
		val, err := resolveFileRef(v)
		if err != nil {
			return "", err
		}
		return val, nil
	default:
		return expandEnv(v)
	}
}

func expandEnv(s string) (string, error) {
	if !envVarPattern.MatchString(s) {
		return s, nil
	}
	var missing bool
	out := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := envVarPattern.FindStringSubmatch(match)[1]
		val, ok := os.LookupEnv(name)
		if !ok || strings.TrimSpace(val) == "" {
			missing = true
			return ""
		}
		return val
	})
	if missing {
		return "", ErrInvalidMCP
	}
	return out, nil
}

func resolveEnvRef(ref string) (string, error) {
	val := strings.TrimSpace(os.Getenv(strings.TrimPrefix(ref, "env:")))
	if val == "" {
		return "", ErrInvalidMCP
	}
	return val, nil
}

func resolveFileRef(ref string) (string, error) {
	path := strings.TrimSpace(strings.TrimPrefix(ref, "file:"))
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return "", ErrInvalidMCP
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", ErrInvalidMCP
	}
	val := strings.TrimSpace(string(data))
	if val == "" {
		return "", ErrInvalidMCP
	}
	return val, nil
}

