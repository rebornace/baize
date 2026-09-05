package workspace_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/workspace"
)

func newSvc(t *testing.T) *workspace.Service {
	t.Helper()
	st, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	// The service defaults to vision-off (safe default for production
	// misconfiguration); these service-level tests exercise the image paths
	// directly, so opt in to a vision-capable model.
	return workspace.New(st, workspace.WithVision(func() bool { return true }))
}

func TestWriteReadListDeleteRoundTrip(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.WriteFile(ctx, "c1", "notes/summary.md", "# hi"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReadFile(ctx, "c1", "notes/summary.md")
	if err != nil || got != "# hi" {
		t.Fatalf("read=%q err=%v", got, err)
	}
	entries, err := svc.ListFiles(ctx, "c1", "")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, e := range entries {
		found[e.Name] = e.Type
	}
	if found["notes"] != "dir" {
		t.Fatalf("want notes dir, got %v", entries)
	}
	sub, _ := svc.ListFiles(ctx, "c1", "notes")
	if len(sub) != 1 || sub[0].Name != "summary.md" || sub[0].Type != "file" {
		t.Fatalf("want single file under notes, got %v", sub)
	}
	if err := svc.DeleteFile(ctx, "c1", "notes/summary.md"); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteFile(ctx, "c1", "notes/missing.md"); err != nil {
		t.Fatalf("delete missing must be idempotent: %v", err)
	}
}

func TestConversationIsolation(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	_ = svc.WriteFile(ctx, "c1", "secret.txt", "topsecret")
	if _, err := svc.ReadFile(ctx, "c2", "secret.txt"); err == nil {
		t.Fatal("c2 must not read c1's file")
	}
	entries, _ := svc.ListFiles(ctx, "c2", "")
	if len(entries) != 0 {
		t.Fatalf("c2 workspace must be empty, got %v", entries)
	}
}

func TestPathTraversalRejected(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()
	if err := svc.WriteFile(ctx, "c1", "../escape.txt", "x"); err == nil {
		t.Fatal("traversal must be rejected")
	}
	if _, err := svc.ReadFile(ctx, "c1", "/etc/passwd"); err == nil {
		t.Fatal("absolute path must be rejected")
	}
}

func TestEmptyConversationRejected(t *testing.T) {
	svc := newSvc(t)
	if err := svc.WriteFile(context.Background(), "", "a.txt", "x"); err == nil {
		t.Fatal("empty convID must error")
	}
}

func TestUnsafeConversationRejected(t *testing.T) {
	// M-4 defense-in-depth: even though convID is server-supplied, every
	// public method must reject IDs that could escape the workspaces/ prefix.
	svc := newSvc(t)
	ctx := context.Background()
	bad := []string{
		"",
		"   ",
		"../x",
		"..",
		"a/b",
		`a\b`,
		".",
		"foo..bar",
	}
	for _, id := range bad {
		if err := svc.WriteFile(ctx, id, "a.txt", "z"); err == nil {
			t.Fatalf("WriteFile with convID %q must be rejected", id)
		}
		if _, err := svc.ReadFile(ctx, id, "a.txt"); err == nil {
			t.Fatalf("ReadFile with convID %q must be rejected", id)
		}
	}
}

func TestWriteTooLarge(t *testing.T) {
	svc := newSvc(t)
	big := strings.Repeat("x", workspace.MaxWriteBytes+1)
	if err := svc.WriteFile(context.Background(), "c1", "big.txt", big); err == nil {
		t.Fatal("oversize write must error")
	}
}

func TestReadTruncates(t *testing.T) {
	svc := newSvc(t)
	// MaxReadBytes (64 KiB) < this length < MaxWriteBytes (256 KiB), so the
	// write is accepted but the read must be truncated at the read cap.
	long := strings.Repeat("y", workspace.MaxReadBytes+100)
	if err := svc.WriteFile(context.Background(), "c1", "long.txt", long); err != nil {
		t.Fatal(err)
	}
	got, err := svc.ReadFile(context.Background(), "c1", "long.txt")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, workspace.TruncatedMarker) {
		t.Fatalf("read must be truncated with marker, len=%d", len(got))
	}
}

func TestSaveUploadReadable(t *testing.T) {
	svc := newSvc(t)
	logical, err := svc.SaveUpload(context.Background(), "c1", "report.pdf", "body text")
	if err != nil {
		t.Fatal(err)
	}
	if logical != "uploads/report.pdf" {
		t.Fatalf("logical path=%q", logical)
	}
	got, err := svc.ReadFile(context.Background(), "c1", logical)
	if err != nil || got != "body text" {
		t.Fatalf("upload not readable at uploads/ path: %q err=%v", got, err)
	}
}

func TestReadImageReturnsPart(t *testing.T) {
	svc := newSvc(t)
	// PNG signature bytes (http.DetectContentType -> image/png).
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	if _, err := svc.SaveUploadBytes(context.Background(), "c1", "shot.png", png, "image/png"); err != nil {
		t.Fatal(err)
	}
	part, ctype, err := svc.ReadImage(context.Background(), "c1", "uploads/shot.png")
	if err != nil {
		t.Fatal(err)
	}
	if part.Type != "image" || len(part.ImageBytes) == 0 {
		t.Fatalf("bad part %+v", part)
	}
	if !strings.HasPrefix(ctype, "image/") {
		t.Fatalf("content type %q not image", ctype)
	}
}

func TestReadImageRejectsText(t *testing.T) {
	svc := newSvc(t)
	_, _ = svc.SaveUpload(context.Background(), "c1", "a.txt", "not an image")
	if _, _, err := svc.ReadImage(context.Background(), "c1", "uploads/a.txt"); err == nil {
		t.Fatal("reading text as image must error")
	}
}

func TestResolveImagePart(t *testing.T) {
	svc := newSvc(t)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}
	_, _ = svc.SaveUploadBytes(context.Background(), "c1", "x.png", png, "image/png")
	part, ok := svc.ResolveImagePart(context.Background(), "c1", "uploads/x.png")
	if !ok || part.Type != "image" {
		t.Fatalf("resolve failed: %+v ok=%v", part, ok)
	}
	if _, ok := svc.ResolveImagePart(context.Background(), "c1", "uploads/missing.png"); ok {
		t.Fatal("missing image must resolve to ok=false")
	}
}
