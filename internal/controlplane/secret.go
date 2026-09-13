package controlplane

import (
	"fmt"
	"os"
	"strings"
)

func ResolveSecret(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	switch {
	case strings.HasPrefix(ref, "env:"):
		return strings.TrimSpace(os.Getenv(strings.TrimPrefix(ref, "env:"))), nil
	case strings.HasPrefix(ref, "file:"):
		path := strings.TrimSpace(strings.TrimPrefix(ref, "file:"))
		info, err := os.Stat(path)
		if err != nil {
			return "", fmt.Errorf("controlplane: read secret file %q: %w", path, err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("controlplane: secret file %q is a directory", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("controlplane: read secret file %q: %w", path, err)
		}
		return strings.TrimSpace(string(data)), nil
	default:
		return ref, nil
	}
}
