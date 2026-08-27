package authcred

import (
	"os"
	"regexp"
	"strings"
)

var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func NormalizeMode(mode string) string {
	if strings.TrimSpace(mode) == "" {
		return ModeStatic
	}
	return strings.TrimSpace(mode)
}

func expandEnv(s string) (string, error) {
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
		return "", ErrInvalidAuth
	}
	return out, nil
}
