//go:build windows

package toolretrieval

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// tryStartLocalOllama launches the Windows GUI app (tray), not the CLI ollama.exe.
// modelsDir, when non-empty, is injected as OLLAMA_MODELS for this process tree.
// Returns true when ProbeOllama succeeds within a short window.
func tryStartLocalOllama(ctx context.Context, baseURL, modelsDir string, onProgress func(string)) bool {
	exe := findLocalOllamaGUI()
	if exe == "" {
		return false
	}
	if onProgress != nil {
		onProgress("starting_local_ollama")
	}
	cmd := exec.Command(exe)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Env = os.Environ()
	if d := strings.TrimSpace(modelsDir); d != "" {
		cmd.Env = append(cmd.Env, "OLLAMA_MODELS="+d)
	}
	_ = cmd.Start()
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	// Cold start of the tray app can take well over a minute on first launch.
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false
		}
		if ProbeOllama(ctx, baseURL) == nil {
			return true
		}
		time.Sleep(2 * time.Second)
	}
	return ProbeOllama(ctx, baseURL) == nil
}

// findLocalOllamaGUI prefers the desktop tray app. Never prefer PATH "ollama"
// (that is the CLI entrypoint and confuses users into thinking there is no UI).
func findLocalOllamaGUI() string {
	local := os.Getenv("LOCALAPPDATA")
	pathCLI, _ := exec.LookPath("ollama")
	for _, c := range ollamaLaunchCandidates(local, pathCLI) {
		if c == "" {
			continue
		}
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	return ""
}

// ollamaLaunchCandidates returns Windows launch paths in preference order:
// GUI tray app first, CLI (including PATH "ollama") last.
func ollamaLaunchCandidates(localAppData, pathCLI string) []string {
	var out []string
	if localAppData != "" {
		dir := filepath.Join(localAppData, "Programs", "Ollama")
		out = append(out,
			filepath.Join(dir, "ollama app.exe"), // GUI / tray
			filepath.Join(dir, "Ollama.exe"),
			filepath.Join(dir, "ollama.exe"), // CLI in install dir
		)
	}
	if pathCLI != "" {
		// Deduplicate if PATH already points at install-dir CLI.
		dup := false
		for _, c := range out {
			if strings.EqualFold(c, pathCLI) {
				dup = true
				break
			}
		}
		if !dup {
			out = append(out, pathCLI)
		}
	}
	return out
}

// downloadAndLaunchInstaller tries region-ordered mirrors (CN: ModelScope first),
// skipping stale sizes, then falls back to opening a browser download link.
func downloadAndLaunchInstaller(ctx context.Context, onProgress func(InstallProgress)) (string, error) {
	mirrors := windowsInstallerMirrors()
	if len(mirrors) == 0 {
		return "", fmt.Errorf("no windows installer mirrors")
	}
	wantSize := expectedInstallerSize(ctx)

	var lastErr error
	attempt := 0
	for _, m := range mirrors {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if attempt > 0 && onProgress != nil {
			note := "上一源失败，改用其他源重新下载（进度会从 0 开始）"
			if lastErr != nil {
				note = fmt.Sprintf("上一源失败（%s），改用其他源重新下载", truncateErr(lastErr))
			}
			onProgress(InstallProgress{
				Event:  "switch_mirror",
				Mirror: m.ID,
				Note:   note,
			})
		}
		attempt++
		dest, err := downloadInstaller(ctx, m.URL, m.ID, wantSize, onProgress)
		if err != nil {
			lastErr = err
			continue
		}
		if onProgress != nil {
			onProgress(InstallProgress{Event: "launch", Mirror: m.ID})
		}
		cmd := exec.Command(dest)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			lastErr = err
			continue
		}
		_ = cmd.Process.Release()
		return dest, nil
	}

	if onProgress != nil {
		onProgress(InstallProgress{Event: "open_page", Note: "自动下载均失败，已打开安装包直链"})
	}
	if len(mirrors) > 0 {
		_ = openURL(mirrors[0].URL)
	} else {
		_ = openDownloadPage()
	}
	return "", nil
}

func truncateErr(err error) string {
	if err == nil {
		return ""
	}
	s := err.Error()
	if len(s) > 80 {
		return s[:80] + "…"
	}
	return s
}

func downloadInstaller(ctx context.Context, url, mirrorID string, wantSize int64, onProgress func(InstallProgress)) (string, error) {
	dir := InstallerCacheDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dest := filepath.Join(dir, "OllamaSetup-"+mirrorID+".exe")

	// Cancel on stall / wall-clock; do NOT use a short absolute Timeout that
	// aborts a healthy multi‑GB download mid-way (that looked like "1000MB → 0").
	dlCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	const stallLimit = 45 * time.Second
	const maxWall = 45 * time.Minute
	started := time.Now()
	var lastDataNano atomic.Int64
	lastDataNano.Store(time.Now().UnixNano())

	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-dlCtx.Done():
				return
			case <-t.C:
				last := time.Unix(0, lastDataNano.Load())
				if time.Since(last) > stallLimit {
					cancel()
					return
				}
				if time.Since(started) > maxWall {
					cancel()
					return
				}
			}
		}
	}()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", installerHTTPUserAgent)
	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 20 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	res, err := client.Do(req)
	if err != nil {
		if dlCtx.Err() != nil && ctx.Err() == nil {
			return "", fmt.Errorf("download stalled or timed out")
		}
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("download status %d", res.StatusCode)
	}

	var total int64
	if cl := res.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > 0 {
			total = n
		}
	}
	if installerSizeLooksStale(total, wantSize) {
		_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 64))
		return "", fmt.Errorf("stale installer size %d (want ~%d)", total, wantSize)
	}

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()

	var written int64
	buf := make([]byte, 256*1024)
	lastReport := time.Time{}
	report := func(force bool) {
		if onProgress == nil {
			return
		}
		if !force && time.Since(lastReport) < 400*time.Millisecond {
			return
		}
		lastReport = time.Now()
		onProgress(InstallProgress{
			Event:  "download",
			Mirror: mirrorID,
			Bytes:  written,
			Total:  total,
		})
	}
	report(true)

	for {
		n, readErr := res.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return "", werr
			}
			written += int64(n)
			lastDataNano.Store(time.Now().UnixNano())
			report(false)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			if dlCtx.Err() != nil && ctx.Err() == nil {
				return "", fmt.Errorf("download stalled after %dMB", written/(1024*1024))
			}
			return "", readErr
		}
	}
	report(true)

	if written < 1024*1024 {
		return "", fmt.Errorf("installer too small (%d bytes)", written)
	}
	if total > 0 && written < total*9/10 {
		return "", fmt.Errorf("incomplete download %d/%d bytes", written, total)
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return dest, nil
}

func openURL(u string) error {
	u = strings.TrimSpace(u)
	if u == "" {
		return fmt.Errorf("empty url")
	}
	return exec.Command("rundll32", "url.dll,FileProtocolHandler", u).Start()
}

func openDownloadPage() error {
	var firstErr error
	// Prefer a direct installer link when available (browser download manager).
	for _, m := range windowsInstallerMirrors() {
		if err := openURL(m.URL); err == nil {
			return nil
		} else if firstErr == nil {
			firstErr = err
		}
	}
	for _, u := range downloadPageFallbacks() {
		err := openURL(u)
		if err == nil {
			return nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
