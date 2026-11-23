package blob_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
)

func TestDeletePrefix(t *testing.T) {
	ctx := context.Background()
	s, err := blob.Open(ctx, "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Put(ctx, "connectors/c1/a", []byte("x"), "")
	_ = s.Put(ctx, "connectors/c1/b", []byte("y"), "")
	_ = s.Put(ctx, "connectors/other/z", []byte("z"), "")
	if err := blob.DeletePrefix(ctx, s, "connectors/c1/"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "connectors/c1/a"); !errors.Is(err, blob.ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, "connectors/other/z"); err != nil {
		t.Fatal(err)
	}
}
