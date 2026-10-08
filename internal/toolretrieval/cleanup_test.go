package toolretrieval

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClearInstallerCache(t *testing.T) {
	dir := InstallerCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "dummy.bin")
	if err := os.WriteFile(path, []byte("hello-cache"), 0o644); err != nil {
		t.Fatal(err)
	}
	n, err := ClearInstallerCache()
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("removed=%d", n)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}
