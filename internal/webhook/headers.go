package webhook

import (
	"os"
	"strings"

	"github.com/rebornace/baize/internal/authcred"
)

// resolveHeaders expands env:/file: and ${VAR} references for outbound delivery.
func resolveHeaders(headers map[string]string) (map[string]string, error) {
	if len(headers) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(headers))
	for k, ref := range headers {
		val, err := resolveHeaderValue(ref)
		if err != nil {
			return nil, err
		}
		out[k] = val
	}
	return out, nil
}

func resolveHeaderValue(ref string) (string, error) {
	switch {
	case strings.HasPrefix(ref, "env:"):
		val := strings.TrimSpace(os.Getenv(strings.TrimPrefix(ref, "env:")))
		if val == "" {
			return "", authcred.ErrInvalidAuth
		}
		return val, nil
	case strings.HasPrefix(ref, "file:"):
		path := strings.TrimSpace(strings.TrimPrefix(ref, "file:"))
		info, err := os.Stat(path)
		if err != nil || info.IsDir() {
			return "", authcred.ErrInvalidAuth
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", authcred.ErrInvalidAuth
		}
		val := strings.TrimSpace(string(data))
		if val == "" {
			return "", authcred.ErrInvalidAuth
		}
		return val, nil
	default:
		return expandEnvVars(ref)
	}
}

func expandEnvVars(s string) (string, error) {
	resolved, err := authcred.ResolveDefaults(authcred.Config{
		Mode:   authcred.ModeStatic,
		Static: authcred.Static{Headers: map[string]string{"_": s}},
	})
	if err != nil {
		return "", err
	}
	return resolved["_"], nil
}
