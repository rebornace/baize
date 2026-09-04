package blob_test

import (
	"context"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/blob"
)

func TestOpenUnknownDriver(t *testing.T) {
	_, err := blob.Open(context.Background(), "nope", blob.Options{})
	if err == nil || !strings.Contains(err.Error(), `unknown blob driver`) {
		t.Fatalf("want unknown driver error, got %v", err)
	}
}

func TestRegisterDuplicatePanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("duplicate registration should panic")
		}
	}()
	blob.RegisterDriver("dup-test", func(_ context.Context, _ blob.Options) (blob.Store, error) {
		return nil, nil
	})
	blob.RegisterDriver("dup-test", func(_ context.Context, _ blob.Options) (blob.Store, error) {
		return nil, nil
	})
}
