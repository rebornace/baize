package toolretrieval

import (
	"runtime"
	"testing"
)

func TestWindowsInstallerMirrorsRegionOrder(t *testing.T) {
	t.Setenv("BAIZE_OLLAMA_MIRROR", "official")
	m := windowsInstallerMirrors()
	if len(m) < 3 {
		t.Fatalf("expected 3 mirrors, got %d", len(m))
	}
	if m[0].ID != "official" {
		t.Fatalf("global first want official, got %q", m[0].ID)
	}
	if preferredDownloadPage() != downloadPageOfficial {
		t.Fatalf("global download page want official, got %q", preferredDownloadPage())
	}

	t.Setenv("BAIZE_OLLAMA_MIRROR", "cn")
	m = windowsInstallerMirrors()
	if m[0].ID != "cn" {
		t.Fatalf("cn first want cn, got %q", m[0].ID)
	}
	if preferredDownloadPage() != downloadPageCN {
		t.Fatalf("cn download page want CN, got %q", preferredDownloadPage())
	}
}

func TestPreferChineseMirrorsEnvOverridesLocale(t *testing.T) {
	t.Setenv("LANG", "zh_CN.UTF-8")
	t.Setenv("BAIZE_OLLAMA_MIRROR", "official")
	if preferChineseMirrors() {
		t.Fatal("BAIZE_OLLAMA_MIRROR=official must win over LANG")
	}
	t.Setenv("BAIZE_OLLAMA_MIRROR", "cn")
	t.Setenv("LANG", "en_US.UTF-8")
	if !preferChineseMirrors() {
		t.Fatal("BAIZE_OLLAMA_MIRROR=cn must win over LANG")
	}
}

func TestEnvLangPrefersChinese(t *testing.T) {
	t.Setenv("BAIZE_OLLAMA_MIRROR", "")
	t.Setenv("LC_ALL", "")
	t.Setenv("LC_MESSAGES", "")
	t.Setenv("LANGUAGE", "")
	t.Setenv("LANG", "zh_CN.UTF-8")
	if !envLangPrefersChinese() {
		t.Fatal("zh_CN should prefer Chinese mirrors")
	}
	t.Setenv("LANG", "en_US.UTF-8")
	if envLangPrefersChinese() {
		t.Fatal("en_US should not prefer Chinese via LANG")
	}
}

func TestInstallerForGOOSListsMirrors(t *testing.T) {
	t.Setenv("BAIZE_OLLAMA_MIRROR", "official")
	hint := InstallerForGOOS()
	if runtime.GOOS == "windows" && len(hint.MirrorURLs) == 0 {
		t.Fatal("windows hint should list mirror_urls")
	}
	if hint.DownloadPageURL != downloadPageOfficial {
		t.Fatalf("with official override, page=%q", hint.DownloadPageURL)
	}
}
