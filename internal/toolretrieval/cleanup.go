package toolretrieval

import (
	"os"
	"path/filepath"
)

// InstallerCacheDir is where baize stores downloaded OllamaSetup.exe mirrors.
func InstallerCacheDir() string {
	return filepath.Join(os.TempDir(), "baize-ollama-setup")
}

// ClearInstallerCache removes downloaded installer files. Returns bytes removed.
func ClearInstallerCache() (int64, error) {
	dir := InstallerCacheDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	var removed int64
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if e.IsDir() {
			_ = os.RemoveAll(path)
			continue
		}
		sz := info.Size()
		if err := os.Remove(path); err == nil {
			removed += sz
		}
	}
	_ = os.Remove(dir) // ok if not empty / fails
	return removed, nil
}

// InstallerCacheBytes returns current cache size (best-effort).
func InstallerCacheBytes() int64 {
	dir := InstallerCacheDir()
	var total int64
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
