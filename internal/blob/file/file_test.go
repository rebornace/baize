package file_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
)

func TestFilePutGetRoundTrip(t *testing.T) {
	root := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: root}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "artifacts/art_abc.html", []byte("<html>ok</html>"), "text/html; charset=utf-8"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "artifacts/art_abc.html")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<html>ok</html>" {
		t.Fatalf("got %q", got)
	}
	// 文件确实落在 <root>/artifacts/ 下（路径与今日布局一致）。
	if _, err := os.Stat(filepath.Join(root, "artifacts", "art_abc.html")); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}
}

func TestFileGetMissing(t *testing.T) {
	root := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: root}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Get(context.Background(), "artifacts/nope.html")
	if !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestFileDeleteIdempotent(t *testing.T) {
	root := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: root}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "a/b/c.bin", []byte("x"), "application/octet-stream"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "a/b/c.bin"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "a/b/c.bin"); err != nil { // 再删不报错
		t.Fatalf("delete missing should be nil, got %v", err)
	}
	if _, err := s.Get(ctx, "a/b/c.bin"); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("want ErrNotFound after delete, got %v", err)
	}
}
