package toolretrieval

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
)

func TestProbeOllamaAndEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models":[]}`))
		case "/v1/embeddings":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"embedding": []float32{0.1, 0.2}}},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	if err := ProbeOllama(context.Background(), srv.URL); err != nil {
		t.Fatal(err)
	}
	if err := ProbeEmbedding(context.Background(), srv.URL+"/v1", "bge-m3", ""); err != nil {
		t.Fatal(err)
	}
}

func TestProbeEmbeddingWithAPIKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"embedding": []float32{0.3, 0.4}}},
		})
	}))
	defer srv.Close()
	if err := ProbeEmbedding(context.Background(), srv.URL, "text-embedding-3-small", "sk-test"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer sk-test" {
		t.Fatalf("auth=%q", gotAuth)
	}
}

func TestNormalizeOpenAIBase(t *testing.T) {
	if got := NormalizeOpenAIBase("https://api.openai.com/v1"); got != "https://api.openai.com/v1" {
		t.Fatalf("got %q", got)
	}
	if got := NormalizeOpenAIBase("https://api.openai.com"); got != "https://api.openai.com/v1" {
		t.Fatalf("got %q", got)
	}
}

func TestOllamaProbeBasesIncludeLocalhost(t *testing.T) {
	bases := ollamaProbeBases("http://127.0.0.1:11434")
	found := false
	for _, b := range bases {
		if b == "http://localhost:11434" {
			found = true
		}
	}
	if !found {
		t.Fatalf("want localhost variant, got %v", bases)
	}
}

func TestDownloadPercent(t *testing.T) {
	if downloadPercent(0, 100) != 0 {
		t.Fatal()
	}
	if downloadPercent(50, 100) != 50 {
		t.Fatal()
	}
	if downloadPercent(100, 100) != 100 {
		t.Fatal()
	}
	if downloadPercent(10, 0) != 0 {
		t.Fatal()
	}
}

func TestWindowsInstallerMirrorsPreferCN(t *testing.T) {
	m := windowsInstallerMirrors()
	if len(m) < 2 {
		t.Fatalf("expected multiple mirrors, got %d", len(m))
	}
	if m[0].ID != "cn" {
		t.Fatalf("first mirror want cn, got %q", m[0].ID)
	}
	hint := InstallerForGOOS()
	if hint.DownloadPageURL == downloadPageOfficial {
		t.Fatalf("download page should prefer CN, got %q", hint.DownloadPageURL)
	}
	if len(hint.MirrorURLs) == 0 && runtime.GOOS == "windows" {
		t.Fatal("windows hint should list mirror_urls")
	}
}

func TestOpenAIBaseFromOllama(t *testing.T) {
	if got := OpenAIBaseFromOllama("http://127.0.0.1:11434"); got != "http://127.0.0.1:11434/v1" {
		t.Fatalf("got %q", got)
	}
	if got := OpenAIBaseFromOllama("http://x/v1"); got != "http://x/v1" {
		t.Fatalf("got %q", got)
	}
}
