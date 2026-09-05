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

func TestFileListByPrefix(t *testing.T) {
	dir := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	must := func(key string, data []byte) {
		t.Helper()
		if err := s.Put(ctx, key, data, ""); err != nil {
			t.Fatal(err)
		}
	}
	must("workspaces/c1/uploads/a.txt", []byte("hello"))
	must("workspaces/c1/uploads/nested/b.txt", []byte("world!"))
	must("workspaces/c1/notes.md", []byte("note"))
	must("workspaces/c2/other.txt", []byte("x"))

	got, err := s.List(ctx, "workspaces/c1/")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{
		"workspaces/c1/uploads/a.txt":        5,
		"workspaces/c1/uploads/nested/b.txt": 6,
		"workspaces/c1/notes.md":             4,
	}
	have := map[string]int64{}
	for _, e := range got {
		have[e.Key] = e.Size
	}
	if len(have) != len(want) {
		t.Fatalf("list count=%d want %d (%v)", len(have), len(want), have)
	}
	for k, sz := range want {
		if have[k] != sz {
			t.Fatalf("key %s size=%d want %d", k, have[k], sz)
		}
	}
}

func TestFileListMissingPrefixEmpty(t *testing.T) {
	dir := t.TempDir()
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.List(context.Background(), "workspaces/nope/")
	if err != nil {
		t.Fatalf("missing prefix must not error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("want empty, got %v", got)
	}
}
