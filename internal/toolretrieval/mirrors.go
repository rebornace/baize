package toolretrieval

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// InstallerMirror is one candidate URL for the Ollama desktop installer.
type InstallerMirror struct {
	ID  string // short id shown in UI progress (e.g. "cn", "official")
	URL string
}

const (
	// Official installer + download page.
	windowsInstallerOfficial = "https://ollama.com/download/OllamaSetup.exe"
	downloadPageOfficial     = "https://ollama.com/download"

	// GitHub releases/latest — same binary as official.
	windowsInstallerGitHub = "https://github.com/ollama/ollama/releases/latest/download/OllamaSetup.exe"

	// ghproxy.net — GitHub latest accelerator (often crawls on CN).
	windowsInstallerGHProxy = "https://ghproxy.net/https://github.com/ollama/ollama/releases/latest/download/OllamaSetup.exe"

	// ghfast — alternate GitHub proxy (fallback).
	windowsInstallerGHFast = "https://ghfast.top/https://github.com/ollama/ollama/releases/latest/download/OllamaSetup.exe"

	// ModelScope community sync (Lixiang/ollama-release) — fast CN CDN.
	modelScopeOwner            = "Lixiang"
	modelScopeRepo             = "ollama-release"
	modelScopeFallbackRevision = "v0.40.1"
	downloadPageModelScope     = "https://www.modelscope.cn/models/Lixiang/ollama-release/files"

	downloadPageCN     = "https://docs.ollama.ac.cn/windows"
	downloadPageCNDocs = "https://docs.ollama.ac.cn/windows"

	// ModelScope rejects empty/odd UAs on the LFS CDN; keep a stable product UA.
	installerHTTPUserAgent = "Baize/1.0 (+https://github.com/rebornace/baize)"

	// Reject mirrors whose Content-Length differs from the reference by more
	// than this (stale community syncs have been ~5MB+ smaller).
	installerSizeSlopBytes = 1024 * 1024
)

var modelScopeStableTagRE = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)

// preferChineseMirrors reports whether CN-friendly accelerators should be tried early.
// Override: BAIZE_OLLAMA_MIRROR=cn|china  or  official|global|intl|en
func preferChineseMirrors() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BAIZE_OLLAMA_MIRROR"))) {
	case "cn", "china", "cnb", "modelscope":
		return true
	case "official", "global", "intl", "international", "en":
		return false
	}
	if envLangPrefersChinese() {
		return true
	}
	return systemLocaleChinese()
}

func envLangPrefersChinese() bool {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG", "LANGUAGE"} {
		v := strings.ToLower(strings.ReplaceAll(os.Getenv(k), "-", "_"))
		if v == "" {
			continue
		}
		if strings.Contains(v, "zh_cn") || strings.Contains(v, "zh_hans") ||
			strings.HasPrefix(v, "zh_") || v == "zh" || strings.HasPrefix(v, "zh.") {
			return true
		}
	}
	return false
}

func modelScopeResolveURL(revision string) string {
	rev := strings.TrimSpace(revision)
	if rev == "" {
		rev = modelScopeFallbackRevision
	}
	return fmt.Sprintf(
		"https://www.modelscope.cn/models/%s/%s/resolve/%s/OllamaSetup.exe",
		modelScopeOwner, modelScopeRepo, rev,
	)
}

func modelScopeRevisionsAPI() string {
	return fmt.Sprintf(
		"https://www.modelscope.cn/api/v1/models/%s/%s/revisions",
		modelScopeOwner, modelScopeRepo,
	)
}

func modelScopeFilesAPI(revision string) string {
	return fmt.Sprintf(
		"https://www.modelscope.cn/api/v1/models/%s/%s/repo/files?Revision=%s&Root=",
		modelScopeOwner, modelScopeRepo, revision,
	)
}

type modelScopeRevisionsResp struct {
	Code int `json:"Code"`
	Data struct {
		RevisionMap struct {
			Tags []struct {
				Revision string `json:"Revision"`
			} `json:"Tags"`
		} `json:"RevisionMap"`
	} `json:"Data"`
}

type modelScopeFilesResp struct {
	Code int `json:"Code"`
	Data struct {
		Files []struct {
			Name string `json:"Name"`
			Size int64  `json:"Size"`
		} `json:"Files"`
	} `json:"Data"`
}

func httpGetJSON(ctx context.Context, url string, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", installerHTTPUserAgent)
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 6 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 2<<20))
	if err != nil {
		return err
	}
	return json.Unmarshal(body, dest)
}

// latestModelScopeStableTag returns the newest vX.Y.Z tag (skips -rc), or "".
func latestModelScopeStableTag(ctx context.Context) string {
	var resp modelScopeRevisionsResp
	if err := httpGetJSON(ctx, modelScopeRevisionsAPI(), &resp); err != nil {
		return ""
	}
	for _, t := range resp.Data.RevisionMap.Tags {
		rev := strings.TrimSpace(t.Revision)
		if modelScopeStableTagRE.MatchString(rev) {
			return rev
		}
	}
	return ""
}

func modelScopeOllamaSetupSize(ctx context.Context, revision string) int64 {
	var resp modelScopeFilesResp
	if err := httpGetJSON(ctx, modelScopeFilesAPI(revision), &resp); err != nil {
		return 0
	}
	for _, f := range resp.Data.Files {
		if strings.EqualFold(f.Name, "OllamaSetup.exe") && f.Size > 0 {
			return f.Size
		}
	}
	return 0
}

// modelScopeWindowsInstaller returns the CN ModelScope mirror for the latest
// stable tag (fallback revision if the API is unreachable).
func modelScopeWindowsInstaller(ctx context.Context) InstallerMirror {
	rev := latestModelScopeStableTag(ctx)
	if rev == "" {
		rev = modelScopeFallbackRevision
	}
	return InstallerMirror{ID: "modelscope", URL: modelScopeResolveURL(rev)}
}

// windowsInstallerMirrors is tried in order.
//
// CN: ModelScope first (Aliyun CDN, tracks GitHub tags closely), then ghproxy /
// official / GitHub / ghfast.
// Global: official → GitHub → ModelScope → accelerators.
func windowsInstallerMirrors() []InstallerMirror {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	official := InstallerMirror{ID: "official", URL: windowsInstallerOfficial}
	github := InstallerMirror{ID: "github", URL: windowsInstallerGitHub}
	ghproxy := InstallerMirror{ID: "cn", URL: windowsInstallerGHProxy}
	ghfast := InstallerMirror{ID: "ghfast", URL: windowsInstallerGHFast}
	modelscope := modelScopeWindowsInstaller(ctx)
	if preferChineseMirrors() {
		return []InstallerMirror{modelscope, ghproxy, official, github, ghfast}
	}
	return []InstallerMirror{official, github, modelscope, ghproxy, ghfast}
}

// preferredDownloadPage returns the first page to open when the browser path is used.
func preferredDownloadPage() string {
	if preferChineseMirrors() {
		return downloadPageModelScope
	}
	return downloadPageOfficial
}

// downloadPageFallbacks lists pages to open when auto-download fails.
func downloadPageFallbacks() []string {
	if preferChineseMirrors() {
		return []string{downloadPageModelScope, downloadPageOfficial, downloadPageCN, downloadPageCNDocs}
	}
	return []string{downloadPageOfficial, downloadPageModelScope, downloadPageCN}
}

// probeInstallerContentLength HEADs url and returns Content-Length, or 0.
func probeInstallerContentLength(ctx context.Context, url string) int64 {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", installerHTTPUserAgent)
	client := &http.Client{Timeout: 6 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return 0
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 400 {
		return 0
	}
	if cl := res.Header.Get("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// expectedInstallerSize returns the Content-Length of a known-fresh installer as
// a freshness reference. Zero means "unknown — skip checks".
func expectedInstallerSize(ctx context.Context) int64 {
	if preferChineseMirrors() {
		rev := latestModelScopeStableTag(ctx)
		if rev == "" {
			rev = modelScopeFallbackRevision
		}
		if n := modelScopeOllamaSetupSize(ctx, rev); n > 0 {
			return n
		}
		if n := probeInstallerContentLength(ctx, modelScopeResolveURL(rev)); n > 0 {
			return n
		}
	}
	candidates := []string{windowsInstallerOfficial, windowsInstallerGitHub, windowsInstallerGHProxy}
	for _, u := range candidates {
		if n := probeInstallerContentLength(ctx, u); n > 0 {
			return n
		}
	}
	return 0
}

func installerSizeLooksStale(got, want int64) bool {
	if want <= 0 || got <= 0 {
		return false
	}
	d := got - want
	if d < 0 {
		d = -d
	}
	return d > installerSizeSlopBytes
}
