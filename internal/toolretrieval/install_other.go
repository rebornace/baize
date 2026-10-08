//go:build !windows

package toolretrieval

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

func tryStartLocalOllama(ctx context.Context, baseURL, modelsDir string, onProgress func(string)) bool {
	_ = ctx
	_ = baseURL
	if p, err := exec.LookPath("ollama"); err == nil {
		if onProgress != nil {
			onProgress("starting_local_ollama")
		}
		cmd := exec.Command(p, "serve")
		cmd.Env = os.Environ()
		if d := strings.TrimSpace(modelsDir); d != "" {
			cmd.Env = append(cmd.Env, "OLLAMA_MODELS="+d)
		}
		_ = cmd.Start()
		if cmd.Process != nil {
			_ = cmd.Process.Release()
		}
	}
	return false
}

func downloadAndLaunchInstaller(ctx context.Context, onProgress func(InstallProgress)) (string, error) {
	_ = ctx
	if onProgress != nil {
		onProgress(InstallProgress{Event: "open_page", Note: "已打开下载页"})
	}
	if err := openDownloadPage(); err != nil {
		return "", err
	}
	return "", nil
}

func openDownloadPage() error {
	pages := downloadPageFallbacks()
	var openCmd func(url string) error
	switch runtime.GOOS {
	case "darwin":
		openCmd = func(url string) error { return exec.Command("open", url).Start() }
	default:
		openCmd = func(url string) error { return exec.Command("xdg-open", url).Start() }
	}
	var firstErr error
	for _, u := range pages {
		if err := openCmd(u); err == nil {
			return nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}
	return fmt.Errorf("open browser to %s", preferredDownloadPage())
}
