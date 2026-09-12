package blob_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
	_ "github.com/rebornace/baize/internal/blob/memory"
)

func TestListDriversIncludesFileAndMemory(t *testing.T) {
	names := blob.ListDrivers()
	got := map[string]bool{}
	for _, n := range names {
		got[n] = true
	}
	if !got["file"] || !got["memory"] {
		t.Fatalf("want file+memory registered, got %v", names)
	}
}

func TestOpenFileViaRegistry(t *testing.T) {
	s, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: t.TempDir()}})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(context.Background(), "x", []byte("y"), ""); err != nil {
		t.Fatal(err)
	}
}
