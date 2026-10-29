package run

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"
)

func identityKnownSecretSample(t *testing.T) string {
	t.Helper()
	// Same accessToken value as internal/identity/redact_test.go TestRedactSensitiveNestedTokens.
	return `{"accessToken":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig"}`
}

func TestFormatOffloadMessageShape(t *testing.T) {
	msg := FormatOffloadMessage("offload/x.txt", "helloSECRET", false)
	if !strings.Contains(msg, "path: offload/x.txt") {
		t.Fatal(msg)
	}
	if !strings.Contains(msg, "preview:") {
		t.Fatal(msg)
	}
	if !IsOffloadPreview(msg) {
		t.Fatal("detector")
	}
}

func TestFormatOffloadMessageRedactsPreview(t *testing.T) {
	raw := identityKnownSecretSample(t)
	msg := FormatOffloadMessage("offload/a.txt", raw, false)
	if strings.Contains(msg, raw) {
		t.Fatal("raw secret leaked into preview")
	}
	if strings.Contains(msg, "eyJ") {
		t.Fatal("jwt prefix leaked into preview")
	}
}

func TestFormatOffloadMessageErrorFirstLine(t *testing.T) {
	msg := FormatOffloadMessage("offload/err.txt", "oops", true)
	first := strings.Split(msg, "\n")[0]
	if !strings.Contains(first, "错误结果") {
		t.Fatalf("first line=%q", first)
	}
	if strings.Contains(first, "可用 read_file 分段读取") {
		t.Fatalf("must not push immediate read_file: %q", first)
	}
	if !IsOffloadPreview(msg) {
		t.Fatal("error stub must be detected as offload preview")
	}
}

func TestFormatOffloadMessagePrefersPreviewOverReadFile(t *testing.T) {
	msg := FormatOffloadMessage("offload/x.txt", "hello", false)
	first := strings.Split(msg, "\n")[0]
	if !strings.Contains(first, "优先根据下方预览") {
		t.Fatalf("first line=%q", first)
	}
	if !IsOffloadPreview(msg) {
		t.Fatal("ok stub must be detected as offload preview")
	}
	// Legacy stubs still detect.
	legacy := "[已卸出到工作区文件，可用 read_file 分段读取]\npath: offload/old.txt\npreview: x"
	if !IsOffloadPreview(legacy) {
		t.Fatal("legacy stub must still detect")
	}
}

func TestIsOffloadPreviewRejectsOldPrunePlaceholder(t *testing.T) {
	if IsOffloadPreview(prunedToolPlaceholder) {
		t.Fatal("old prune text must not look like offload")
	}
}

func TestOffloadPreviewPath(t *testing.T) {
	msg := FormatOffloadMessage("offload/paste-1.txt", "body", false)
	if got := OffloadPreviewPath(msg); got != "offload/paste-1.txt" {
		t.Fatalf("path=%q", got)
	}
	if OffloadPreviewPath("plain") != "" {
		t.Fatal("plain text must yield empty path")
	}
}

func TestMaybeOffloadPasteReplacesLargeText(t *testing.T) {
	ws := newMemWorkspace(t)
	ctx := context.Background()
	large := strings.Repeat("p", PasteOffloadThreshold)
	if utf8.RuneCountInString(large) < PasteOffloadThreshold {
		t.Fatalf("fixture rune count=%d", utf8.RuneCountInString(large))
	}
	out := MaybeOffloadPaste(ctx, ws, "conv-paste", large, true)
	if !IsOffloadPreview(out) {
		snip := out
		if len(snip) > 80 {
			snip = snip[:80]
		}
		t.Fatalf("large paste not offloaded: %q", snip)
	}
	path := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "path: ") {
			path = strings.TrimPrefix(line, "path: ")
			break
		}
	}
	if path == "" {
		t.Fatalf("missing path in stub: %q", out)
	}
	got, err := ws.ReadFile(ctx, "conv-paste", path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if got != large {
		t.Fatalf("workspace body len=%d want %d", len(got), len(large))
	}

	short := "hello paste"
	if MaybeOffloadPaste(ctx, ws, "conv-paste", short, true) != short {
		t.Fatalf("short text must pass through unchanged")
	}
}

func TestMaybeOffloadPasteRedactsFileAndPreview(t *testing.T) {
	ws := newMemWorkspace(t)
	ctx := context.Background()
	// Valid JSON so redactOffloadBody takes the JSON-aware path (same as tool offload).
	large := `{"accessToken":"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.payload.sig","pad":"` +
		strings.Repeat("x", PasteOffloadThreshold) + `"}`
	out := MaybeOffloadPaste(ctx, ws, "conv-secret", large, true)
	if !IsOffloadPreview(out) {
		snip := out
		if len(snip) > 80 {
			snip = snip[:80]
		}
		t.Fatalf("expected offload stub, got %q", snip)
	}
	if strings.Contains(out, "eyJ") {
		t.Fatal("jwt leaked into paste offload preview")
	}
	path := ""
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "path: ") {
			path = strings.TrimPrefix(line, "path: ")
			break
		}
	}
	if path == "" {
		t.Fatalf("missing path in stub: %q", out)
	}
	got, err := ws.ReadFile(ctx, "conv-secret", path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if strings.Contains(got, "eyJ") {
		snip := got
		if len(snip) > 120 {
			snip = snip[:120]
		}
		t.Fatalf("jwt leaked into offload file: %s", snip)
	}
}
