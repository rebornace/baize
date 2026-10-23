package mobileagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRemoteConfigJSON(t *testing.T) {
	a := New()
	_ = a.Configure("https://old.example/v1", "old-key", "old-model")
	raw := a.ApplyRemoteConfigJSON(`{"api_base":"https://new.example/v1","model":"new-model"}`)
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" || resp["api_base"] != "https://new.example/v1" || resp["model"] != "new-model" {
		t.Fatalf("%v", resp)
	}
	if !a.HasAPIKey() {
		t.Fatal("key should remain")
	}
}

func TestFetchRemoteConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"api_base": "https://cfg.example/v1",
			"model":    "cfg-model",
			"api_key":  "cfg-key",
		})
	}))
	t.Cleanup(srv.Close)

	a := New()
	_ = a.Configure("https://x/v1", "", "m")
	raw := a.FetchRemoteConfig(srv.URL)
	var resp map[string]any
	_ = json.Unmarshal([]byte(raw), &resp)
	if resp["status"] != "done" || resp["fetched"] != true {
		t.Fatalf("%v", resp)
	}
	if resp["model"] != "cfg-model" || resp["has_key"] != true {
		t.Fatalf("%v", resp)
	}
}

func TestGoDoesNotPersistAPIKeyToDisk(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	a := New()
	_ = a.Configure("https://example/v1", "super-secret-key-xyz", "m")
	_ = a.ApplyRemoteConfigJSON(`{"api_key":"another-secret-key-abc"}`)

	var leaked []string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		s := string(b)
		if strings.Contains(s, "super-secret-key-xyz") || strings.Contains(s, "another-secret-key-abc") {
			leaked = append(leaked, path)
		}
		return nil
	})
	if len(leaked) > 0 {
		t.Fatalf("api key written to disk: %v", leaked)
	}
}
