package toolretrieval

// InstallerMirror is one candidate URL for the Ollama desktop installer.
type InstallerMirror struct {
	ID  string // short id shown in UI progress (e.g. "cn", "official")
	URL string
}

const (
	// Official installer + download page (often slow or blocked in CN).
	windowsInstallerOfficial = "https://ollama.com/download/OllamaSetup.exe"
	downloadPageOfficial     = "https://ollama.com/download"

	// CNB (Tencent Cloud native build) syncs official GitHub releases — preferred in CN.
	windowsInstallerCNB = "https://cnb.cool/hex/ollama/-/releases/latest/download/OllamaSetup.exe"
	downloadPageCN      = "https://cnb.cool/hex/ollama"
	downloadPageCNDocs  = "https://docs.ollama.ac.cn/windows"

	// GitHub releases (may also time out without a proxy).
	windowsInstallerGitHub = "https://github.com/ollama/ollama/releases/latest/download/OllamaSetup.exe"
)

// windowsInstallerMirrors is tried in order. CN mirror first for domestic networks.
func windowsInstallerMirrors() []InstallerMirror {
	return []InstallerMirror{
		{ID: "cn", URL: windowsInstallerCNB},
		{ID: "official", URL: windowsInstallerOfficial},
		{ID: "github", URL: windowsInstallerGitHub},
	}
}

// preferredDownloadPage returns a page that is reachable in CN when possible.
func preferredDownloadPage() string {
	return downloadPageCN
}

// downloadPageFallbacks lists pages to open when auto-download fails.
func downloadPageFallbacks() []string {
	return []string{downloadPageCN, downloadPageCNDocs, downloadPageOfficial}
}
