package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyPendingSQLiteRestore(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "baize.db")
	if err := os.WriteFile(db, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db+".restore", []byte("new"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(db+".restore-pending", []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := applyPendingSQLiteRestore("sqlite", db); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(db)
	if err != nil || string(got) != "new" {
		t.Fatalf("db=%q err=%v", got, err)
	}
	if _, err := os.Stat(db + ".restore-pending"); !os.IsNotExist(err) {
		t.Fatalf("marker should be removed: %v", err)
	}
	matches, _ := filepath.Glob(db + ".pre-restore-*")
	if len(matches) != 1 {
		t.Fatalf("backup matches=%v", matches)
	}
}
