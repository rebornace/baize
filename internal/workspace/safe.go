// Package workspace provides a per-conversation persistent file workspace
// backed by a blob.Store. Files live under the "workspaces/<convID>/" prefix;
// agents interact through relative logical paths via built-in tools.
package workspace

import (
	"fmt"
	"path"
	"strings"
	"unicode"
)

// safeRelPath validates and normalizes a caller-supplied logical path. It
// returns a clean, slash-separated relative path confined to the workspace
// root: absolute paths, drive letters, empty paths, and any ".." traversal
// after cleaning are rejected.
func safeRelPath(p string) (string, error) {
	// Unconditionally normalize backslashes so behavior is identical on
	// Windows and POSIX (filepath.ToSlash is a no-op on Linux/macOS).
	p = strings.ReplaceAll(p, "\\", "/")
	p = path.Clean(p)
	if p == "" || p == "." {
		return "", fmt.Errorf("path must not be empty")
	}
	if path.IsAbs(p) || strings.HasPrefix(p, "/") {
		return "", fmt.Errorf("absolute paths are not allowed: %s", p)
	}
	// Windows drive letter (e.g. "C:/x").
	if len(p) >= 2 && p[1] == ':' {
		return "", fmt.Errorf("drive-letter paths are not allowed: %s", p)
	}
	if p == ".." || strings.HasPrefix(p, "../") {
		return "", fmt.Errorf("path traversal is not allowed: %s", p)
	}
	return p, nil
}

// sanitizeName reduces an upload filename to a safe base name (no directory
// components, no control characters, no traversal). It never returns empty.
func sanitizeName(name string) string {
	// Normalize backslashes, then take the OS-independent base name via
	// path.Base (filepath.Base only splits on "/" on POSIX).
	name = path.Base(strings.ReplaceAll(name, "\\", "/"))
	var b strings.Builder
	for _, r := range name {
		if unicode.IsControl(r) || r == '/' || r == '\\' {
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." || out == ".." {
		return "file"
	}
	return out
}
