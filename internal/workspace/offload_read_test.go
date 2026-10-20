package workspace_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/workspace"
)

func TestWriteOffloadFileAllowsUpTo2MiB(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	big := strings.Repeat("a", workspace.MaxOffloadBytes)
	path, err := svc.WriteOffloadFile(ctx, "c1", big)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(path, "offload/") || !strings.HasSuffix(path, ".txt") {
		t.Fatalf("path=%q", path)
	}
	// ReadFile caps at MaxReadBytes; verify full payload via ranged reads.
	var rebuilt strings.Builder
	off := 0
	for {
		got, err := svc.ReadFileOpts(ctx, "c1", path, workspace.ReadOpts{Offset: off, Limit: workspace.MaxReadBytes})
		if err != nil {
			t.Fatalf("read chunk at %d: %v", off, err)
		}
		content := got.Content
		if idx := strings.Index(content, "…[truncated"); idx >= 0 {
			content = content[:idx]
		}
		rebuilt.WriteString(content)
		if !got.Truncated {
			break
		}
		off = got.NextOffset
	}
	if rebuilt.String() != big {
		t.Fatalf("roundtrip len=%d want=%d", rebuilt.Len(), len(big))
	}
}

func TestWriteOffloadFileRejectsOverLimit(t *testing.T) {
	svc := newSvc(t)
	_, err := svc.WriteOffloadFile(context.Background(), "c1", strings.Repeat("b", workspace.MaxOffloadBytes+1))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadFileOffsetLimit(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.WriteFile(ctx, "c1", "notes/a.txt", "abcdefghij"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReadFileOpts(ctx, "c1", "notes/a.txt", workspace.ReadOpts{Offset: 3, Limit: 4})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got.Content, "defg") {
		t.Fatalf("content=%q", got.Content)
	}
	if !got.Truncated || got.NextOffset != 7 {
		t.Fatalf("meta=%+v", got)
	}
	full, err := svc.ReadFile(ctx, "c1", "notes/a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(full, "truncated") {
		t.Fatalf("small file must not be truncated: %q", full)
	}
	if full != "abcdefghij" {
		t.Fatalf("full=%q", full)
	}
}
