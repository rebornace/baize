//go:build !windows

package toolretrieval

import "os/exec"

// OllamaInstalled reports whether the ollama binary is on PATH.
func OllamaInstalled() bool {
	_, err := exec.LookPath("ollama")
	return err == nil
}
