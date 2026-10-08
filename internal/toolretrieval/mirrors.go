package toolretrieval

import (
	"os"
	"strings"
)

// InstallerMirror is one candidate URL for the Ollama desktop installer.
type InstallerMirror struct {
	ID  string // short id shown in UI progress (e.g. "cn", "official")
	URL string
}

const (
	// Official installer + download page (default outside mainland China).
	windowsInstallerOfficial = "https://ollama.com/download/OllamaSetup.exe"
	downloadPageOfficial     = "https://ollama.com/download"

	// CNB syncs official GitHub releases — preferred on mainland CN networks.
	windowsInstallerCNB = "https://cnb.cool/hex/ollama/-/releases/latest/download/OllamaSetup.exe"
	downloadPageCN      = "https://cnb.cool/hex/ollama"
	downloadPageCNDocs  = "https://docs.ollama.ac.cn/windows"

	// GitHub releases (may time out without a proxy in some networks).
	windowsInstallerGitHub = "https://github.com/ollama/ollama/releases/latest/download/OllamaSetup.exe"
)

// preferChineseMirrors reports whether CN mirrors should be tried first.
// Order: BAIZE_OLLAMA_MIRROR env → process language → OS UI language.
// Override: cn|china|cnb  or  official|global|intl|en
func preferChineseMirrors() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BAIZE_OLLAMA_MIRROR"))) {
	case "cn", "china", "cnb":
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
		// Prefer mainland signals; bare "zh" still counts (common on CN images).
		if strings.Contains(v, "zh_cn") || strings.Contains(v, "zh_hans") ||
			strings.HasPrefix(v, "zh_") || v == "zh" || strings.HasPrefix(v, "zh.") {
			return true
		}
	}
	return false
}

// windowsInstallerMirrors is tried in order. Mainland CN → CN first;
// elsewhere → official, then GitHub, then CN as last resort.
func windowsInstallerMirrors() []InstallerMirror {
	cn := InstallerMirror{ID: "cn", URL: windowsInstallerCNB}
	official := InstallerMirror{ID: "official", URL: windowsInstallerOfficial}
	github := InstallerMirror{ID: "github", URL: windowsInstallerGitHub}
	if preferChineseMirrors() {
		return []InstallerMirror{cn, official, github}
	}
	return []InstallerMirror{official, github, cn}
}

// preferredDownloadPage returns the first page to open when the browser path is used.
func preferredDownloadPage() string {
	if preferChineseMirrors() {
		return downloadPageCN
	}
	return downloadPageOfficial
}

// downloadPageFallbacks lists pages to open when auto-download fails.
func downloadPageFallbacks() []string {
	if preferChineseMirrors() {
		return []string{downloadPageCN, downloadPageCNDocs, downloadPageOfficial}
	}
	return []string{downloadPageOfficial, downloadPageCN, downloadPageCNDocs}
}
