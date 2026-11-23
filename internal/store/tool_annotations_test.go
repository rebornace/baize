package store_test

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestToolAnnotationsSQLiteRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tools-ann.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}

	readOnly := true
	destructive := false
	s.UpsertConnector(store.Connector{ID: "c1", Type: "mcp"})
	s.UpsertTool(store.Tool{
		ConnectorID:     "c1",
		Name:            "safe_read",
		Source:          store.ToolSourceMCP,
		Enabled:         true,
		ReadOnlyHint:    &readOnly,
		DestructiveHint: &destructive,
	})

	if c, ok := s.(io.Closer); ok {
		_ = c.Close()
	}

	s2, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s2.(io.Closer); ok {
			_ = c.Close()
		}
	})

	got, err := s2.GetTool("safe_read")
	if err != nil {
		t.Fatal(err)
	}
	if got.ReadOnlyHint == nil || !*got.ReadOnlyHint {
		t.Fatalf("ReadOnlyHint=%v want true", got.ReadOnlyHint)
	}
	if got.DestructiveHint == nil || *got.DestructiveHint {
		t.Fatalf("DestructiveHint=%v want false", got.DestructiveHint)
	}
}
