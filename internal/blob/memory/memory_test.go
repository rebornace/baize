package memory_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
)

func TestMemoryPutGetDelete(t *testing.T) {
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err := s.Put(ctx, "k1", []byte("v1"), "text/plain"); err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, "k1")
	if err != nil || string(got) != "v1" {
		t.Fatalf("get=%q err=%v", got, err)
	}
	if _, err := s.Get(ctx, "missing"); !errors.Is(err, blob.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := s.Delete(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, "k1"); err != nil { // 幂等
		t.Fatalf("delete missing should be nil, got %v", err)
	}
}

func TestMemoryListFiltersByPrefix(t *testing.T) {
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	_ = s.Put(ctx, "workspaces/c1/a.txt", []byte("ab"), "")
	_ = s.Put(ctx, "workspaces/c1/b.txt", []byte("cde"), "")
	_ = s.Put(ctx, "workspaces/c2/z.txt", []byte("z"), "")
	got, err := s.List(ctx, "workspaces/c1/")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2, got %v", got)
	}
	for _, e := range got {
		if !strings.HasPrefix(e.Key, "workspaces/c1/") {
			t.Fatalf("key leaked across prefix: %s", e.Key)
		}
	}
}
