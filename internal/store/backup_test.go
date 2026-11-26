package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/store"
)

func TestSQLiteBackupTo(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open("sqlite", filepath.Join(dir, "source.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if c, ok := st.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()
	st.UpsertAgent(store.Agent{ID: "a", System: "s"})
	backup := filepath.Join(dir, "backup.db")
	if err := st.BackupTo(backup); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(backup)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	header := make([]byte, 16)
	if _, err := f.Read(header); err != nil {
		t.Fatal(err)
	}
	if string(header) != "SQLite format 3\x00" {
		t.Fatalf("header=%q", header)
	}
}

func TestMemoryBackupUnsupported(t *testing.T) {
	st := store.NewMemory()
	if err := st.BackupTo(filepath.Join(t.TempDir(), "x.db")); !errors.Is(err, store.ErrBackupUnsupported) {
		t.Fatalf("err=%v", err)
	}
}
