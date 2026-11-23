//go:build windows

package toolretrieval

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestOllamaLaunchCandidatesPreferGUI(t *testing.T) {
	local := `C:\Users\me\AppData\Local`
	pathCLI := filepath.Join(local, "Programs", "Ollama", "ollama.exe")
	c := ollamaLaunchCandidates(local, pathCLI)
	if len(c) < 1 {
		t.Fatal("empty")
	}
	if !strings.HasSuffix(strings.ToLower(c[0]), "ollama app.exe") {
		t.Fatalf("first want GUI app, got %q", c[0])
	}
	// PATH CLI must not come before GUI
	for i, p := range c {
		if strings.EqualFold(filepath.Base(p), "ollama.exe") && i == 0 {
			t.Fatalf("CLI must not be first: %v", c)
		}
	}
}
