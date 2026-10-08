//go:build !windows

package toolretrieval

import (
	"context"
	"fmt"
	"os/exec"
)

func stopLocalOllama() {
	_ = exec.Command("pkill", "-f", "ollama").Run()
}

// UninstallOllama is best-effort on non-Windows: stop the service if possible.
// Full package removal varies by distro and is left to the user.
func UninstallOllama(ctx context.Context) error {
	_ = ctx
	stopLocalOllama()
	return fmt.Errorf("automatic Ollama uninstall is only supported on Windows; stop the app and remove it from system settings")
}
