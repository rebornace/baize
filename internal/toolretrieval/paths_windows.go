//go:build windows

package toolretrieval

import "path/filepath"

func resolveAppDir() string {
	if gui := findLocalOllamaGUI(); gui != "" {
		return filepath.Dir(gui)
	}
	return DefaultAppDir()
}
