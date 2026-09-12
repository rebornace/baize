package channelmedia

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
)

// PNG magic bytes so http.DetectContentType reports image/png.
var pngBytes = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatalf("open memory blob: %v", err)
	}
	return New(blobs)
}

func TestSaveAndOpenImageRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	conv := "weixin:acc@example:peer123" // contains ':' illegal in Windows filenames

	url, obj, err := s.SaveInboundImage(ctx, conv, "shot.png", "image/png", pngBytes)
	if err != nil {
		t.Fatalf("SaveInboundImage: %v", err)
	}
	if !strings.HasPrefix(url, "/v0/channels/media/"+conv+"/") {
		t.Fatalf("url = %q, want it to carry the real conv id for ACL", url)
	}
	if !strings.HasSuffix(obj, ".png") {
		t.Fatalf("object = %q, want .png extension", obj)
	}

	got, mime, found, err := s.OpenMedia(ctx, conv, obj)
	if err != nil || !found {
		t.Fatalf("OpenMedia found=%v err=%v", found, err)
	}
	if mime != "image/png" {
		t.Fatalf("mime = %q, want image/png", mime)
	}
	if string(got) != string(pngBytes) {
		t.Fatal("image bytes mismatch")
	}
}

func TestOpenMediaMissing(t *testing.T) {
	s := newTestStore(t)
	_, _, found, err := s.OpenMedia(context.Background(), "c", "doesnotexist.png")
	if err != nil {
		t.Fatalf("missing object must not error: %v", err)
	}
	if found {
		t.Fatal("found=true for missing object")
	}
}

func TestOpenMediaRejectsTraversal(t *testing.T) {
	s := newTestStore(t)
	for _, bad := range []string{"../x.png", "a/b.png", "..", ".", "x\\y.png"} {
		if _, _, _, err := s.OpenMedia(context.Background(), "c", bad); err == nil {
			t.Fatalf("object %q must be rejected", bad)
		}
	}
}

func TestSaveAndOpenFileDownload(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	docx := []byte{0x50, 0x4B, 0x03, 0x04, 0, 0, 0, 0} // ZIP/OOXML magic
	url, obj, err := s.SaveInboundFile(ctx, "weixin:a:p", "黄山三日行程.docx",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document", docx)
	if err != nil {
		t.Fatalf("SaveInboundFile: %v", err)
	}
	if !strings.HasSuffix(obj, ".docx") {
		t.Fatalf("object = %q, want .docx extension preserved for download", obj)
	}
	if !strings.HasPrefix(url, "/v0/channels/media/") {
		t.Fatalf("url = %q", url)
	}
	got, _, found, err := s.OpenMedia(ctx, "weixin:a:p", obj)
	if err != nil || !found {
		t.Fatalf("OpenMedia found=%v err=%v", found, err)
	}
	if string(got) != string(docx) {
		t.Fatal("file bytes mismatch")
	}
}

func TestSaveInboundFileRejectsOversize(t *testing.T) {
	s := newTestStore(t)
	big := make([]byte, MaxFileBytes+1)
	if _, _, err := s.SaveInboundFile(context.Background(), "c", "big.bin", "application/octet-stream", big); err == nil {
		t.Fatal("oversize file must be rejected")
	}
}

func TestSafeConvSegment(t *testing.T) {
	got := safeConvSegment("weixin:acc:peer-1_x")
	if strings.ContainsAny(got, ":") {
		t.Fatalf("segment %q still contains ':'", got)
	}
	if got != "weixin_acc_peer-1_x" {
		t.Fatalf("segment = %q", got)
	}
}
