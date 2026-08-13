package authcred

import (
	"os"
	"strings"
)

func resolveVaultRef(ref string) (string, error) {
	switch {
	case strings.HasPrefix(ref, "env:"):
		val := strings.TrimSpace(os.Getenv(strings.TrimPrefix(ref, "env:")))
		if val == "" {
			return "", ErrInvalidAuth
		}
		return val, nil
	case strings.HasPrefix(ref, "file:"):
		path := strings.TrimSpace(strings.TrimPrefix(ref, "file:"))
		info, err := os.Stat(path)
		if err != nil {
			return "", ErrInvalidAuth
		}
		if info.IsDir() {
			return "", ErrInvalidAuth
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", ErrInvalidAuth
		}
		val := strings.TrimSpace(string(data))
		if val == "" {
			return "", ErrInvalidAuth
		}
		return val, nil
	default:
		return "", ErrInvalidAuth
	}
}
