//go:build windows

package toolretrieval

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// stopLocalOllama kills tray/CLI processes so uninstall or path changes can apply.
func stopLocalOllama() {
	for _, name := range []string{"ollama app.exe", "ollama.exe", "Ollama.exe"} {
		_ = exec.Command("taskkill", "/IM", name, "/F").Run()
	}
	time.Sleep(500 * time.Millisecond)
}

// UninstallOllama removes the Windows desktop app and leftover data dirs.
// Models/config under the user .ollama folder are also removed.
func UninstallOllama(ctx context.Context) error {
	_ = ctx
	stopLocalOllama()

	appDir := DefaultAppDir()
	if gui := findLocalOllamaGUI(); gui != "" {
		appDir = filepath.Dir(gui)
	}
	unins := filepath.Join(appDir, "unins000.exe")
	if st, err := os.Stat(unins); err == nil && !st.IsDir() {
		cmd := exec.Command(unins, "/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART")
		_ = cmd.Start()
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-done:
			if err != nil {
				// Fall through to manual cleanup.
				_ = err
			}
		case <-time.After(3 * time.Minute):
			_ = cmd.Process.Kill()
		}
	}

	home, _ := os.UserHomeDir()
	local := os.Getenv("LOCALAPPDATA")
	leftovers := []string{
		appDir,
		filepath.Join(local, "Ollama"),
		filepath.Join(home, ".ollama"),
	}
	var last error
	for _, d := range leftovers {
		if d == "" {
			continue
		}
		if err := os.RemoveAll(d); err != nil {
			last = err
		}
	}
	if OllamaInstalled() {
		if last != nil {
			return fmt.Errorf("uninstall incomplete: %w", last)
		}
		return fmt.Errorf("uninstall incomplete: ollama still present")
	}
	return nil
}
