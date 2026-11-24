package attach

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"
)

const maxZipEntries = 200
const maxZipTextBytes = 2 << 20

var zipTextExt = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".xml": true, ".yaml": true, ".yml": true, ".html": true, ".htm": true,
	".log": true, ".ini": true, ".conf": true,
}

// extractZip lists a ZIP without extracting binaries to disk. Text-like entries
// are read up to a bounded budget; other entries are listed by name only.
func extractZip(data []byte) (string, error) {
	if err := checkZipExpansion(data); err != nil {
		return "", err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", err
	}
	var b strings.Builder
	readBytes := 0
	entries := 0
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
		if strings.HasPrefix(name, "../") || strings.HasPrefix(name, "/") || name == ".." {
			continue
		}
		if entries >= maxZipEntries {
			b.WriteString("\n…[zip entry list truncated]")
			break
		}
		entries++
		if !zipTextExt[strings.ToLower(path.Ext(name))] {
			fmt.Fprintf(&b, "- %s (binary, not extracted)\n", name)
			continue
		}
		if readBytes >= maxZipTextBytes {
			fmt.Fprintf(&b, "- %s (text budget exhausted)\n", name)
			continue
		}
		rc, oerr := f.Open()
		if oerr != nil {
			fmt.Fprintf(&b, "- %s (unreadable)\n", name)
			continue
		}
		remaining := maxZipTextBytes - readBytes
		raw, rerr := io.ReadAll(io.LimitReader(rc, int64(remaining)+1))
		_ = rc.Close()
		if rerr != nil {
			fmt.Fprintf(&b, "- %s (unreadable)\n", name)
			continue
		}
		if len(raw) > remaining {
			raw = raw[:remaining]
		}
		readBytes += len(raw)
		text := strings.TrimSpace(string(raw))
		if text == "" {
			continue
		}
		fmt.Fprintf(&b, "## %s\n%s\n\n", name, text)
	}
	return strings.TrimSpace(b.String()), nil
}
