package toolretrieval

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWindowsInstallerMirrorsRegionOrder(t *testing.T) {
	t.Setenv("BAIZE_OLLAMA_MIRROR", "official")
	m := windowsInstallerMirrors()
	if len(m) < 3 {
		t.Fatalf("expected >=3 mirrors, got %d", len(m))
	}
	if m[0].ID != "official" {
		t.Fatalf("global first want official, got %q", m[0].ID)
	}
	if preferredDownloadPage() != downloadPageOfficial {
		t.Fatalf("download page want official, got %q", preferredDownloadPage())
	}

	t.Setenv("BAIZE_OLLAMA_MIRROR", "cn")
	m = windowsInstallerMirrors()
	if m[0].ID != "modelscope" {
		t.Fatalf("cn first want modelscope, got %q", m[0].ID)
	}
	if !strings.Contains(m[0].URL, "modelscope.cn") || !strings.Contains(m[0].URL, "OllamaSetup.exe") {
		t.Fatalf("cn first URL want ModelScope resolve, got %q", m[0].URL)
	}
	if preferredDownloadPage() != downloadPageModelScope {
		t.Fatalf("cn download page want ModelScope, got %q", preferredDownloadPage())
	}
	for _, mir := range m {
		if strings.Contains(mir.URL, "cnb.cool") {
			t.Fatal("stale CNB mirror must not be in the default list")
		}
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
	t.Setenv("BAIZE_OLLAMA_MIRROR", "modelscope")
	if !preferChineseMirrors() {
		t.Fatal("BAIZE_OLLAMA_MIRROR=modelscope should prefer CN")
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

func TestInstallerSizeLooksStale(t *testing.T) {
	want := int64(1578290696)
	if installerSizeLooksStale(want, want) {
		t.Fatal("same size must not be stale")
	}
	if !installerSizeLooksStale(1573069568, want) {
		t.Fatal("older build with different size must look stale")
	}
	if installerSizeLooksStale(0, want) {
		t.Fatal("unknown got size must not reject")
	}
}

func TestModelScopeResolveURL(t *testing.T) {
	u := modelScopeResolveURL("v0.40.1")
	if !strings.Contains(u, "Lixiang/ollama-release/resolve/v0.40.1/OllamaSetup.exe") {
		t.Fatalf("unexpected url %q", u)
	}
	if modelScopeResolveURL("") != modelScopeResolveURL(modelScopeFallbackRevision) {
		t.Fatal("empty rev should use fallback")
	}
}

func TestLatestModelScopeStableTagLive(t *testing.T) {
	if testing.Short() {
		t.Skip("network")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tag := latestModelScopeStableTag(ctx)
	if tag == "" {
		t.Skip("ModelScope unreachable")
	}
	if !modelScopeStableTagRE.MatchString(tag) {
		t.Fatalf("unstable tag %q", tag)
	}
	size := modelScopeOllamaSetupSize(ctx, tag)
	if size < 100*1024*1024 {
		t.Fatalf("OllamaSetup.exe size too small: %d", size)
	}
}
