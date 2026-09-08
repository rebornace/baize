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

	got, mime, found, err := s.OpenImage(ctx, conv, obj)
	if err != nil || !found {
		t.Fatalf("OpenImage found=%v err=%v", found, err)
	}
	if mime != "image/png" {
		t.Fatalf("mime = %q, want image/png", mime)
	}
	if string(got) != string(pngBytes) {
		t.Fatal("image bytes mismatch")
	}
}

func TestOpenImageMissing(t *testing.T) {
	s := newTestStore(t)
	_, _, found, err := s.OpenImage(context.Background(), "c", "doesnotexist.png")
	if err != nil {
		t.Fatalf("missing image must not error: %v", err)
	}
	if found {
		t.Fatal("found=true for missing object")
	}
}

func TestOpenImageRejectsTraversal(t *testing.T) {
	s := newTestStore(t)
	for _, bad := range []string{"../x.png", "a/b.png", "..", ".", "x\\y.png"} {
		if _, _, _, err := s.OpenImage(context.Background(), "c", bad); err == nil {
			t.Fatalf("object %q must be rejected", bad)
		}
	}
}

func TestOpenImageRejectsNonImage(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// Save plain text bytes under a .png name; detection must flag it.
	_, obj, err := s.SaveInboundImage(ctx, "c", "note.png", "image/png", []byte("just plain text, not an image"))
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, _, _, err := s.OpenImage(ctx, "c", obj); err == nil {
		t.Fatal("non-image bytes must be rejected on open")
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
