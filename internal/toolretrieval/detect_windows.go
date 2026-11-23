//go:build windows

package toolretrieval

// OllamaInstalled reports whether the desktop/CLI install is present on disk.
func OllamaInstalled() bool {
	return findLocalOllamaGUI() != ""
}
