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
	a := newTestAgent()
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

	a := newTestAgent()
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

func TestApplyRemoteConfigReadonlyInline(t *testing.T) {
	a := newTestAgent()
	if err := a.Configure("https://old.example/v1", "old-key", "old-model"); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{
		"api_base": "https://new.example/v1",
		"model":    "new-model",
		"readonly_api": map[string]any{
			"base_url":    "https://api.example.com",
			"auth_header": "X-API-Key",
			"auth_value":  "secret-auth-value",
			"spec":        readonlySpecJSON,
		},
	})
	raw := a.ApplyRemoteConfigJSON(string(body))
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" || resp["readonly_tools"] != float64(2) {
		t.Fatalf("resp=%v", resp)
	}
	if a.RemoteToolCount() != 2 {
		t.Fatalf("tools=%d", a.RemoteToolCount())
	}
	for _, leak := range []string{"secret-auth-value", "openapi", "readonlySpec"} {
		if strings.Contains(raw, leak) {
			t.Fatalf("response leaked %q: %s", leak, raw)
		}
	}
}

func TestApplyRemoteConfigReadonlyAPIs(t *testing.T) {
	a := newTestAgent()
	body, _ := json.Marshal(map[string]any{
		"api_base": "https://new.example/v1",
		"model":    "new-model",
		"readonly_apis": []map[string]any{
			{"id": "tickets", "base_url": "https://tickets.example.com", "spec": readonlySpecJSON},
			{"id": "health", "base_url": "https://health.example.com", "spec": readonlyHealthSpecJSON},
		},
	})
	raw := a.ApplyRemoteConfigJSON(string(body))
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" || resp["readonly_tools"] != float64(3) || resp["readonly_connectors"] != float64(2) {
		t.Fatalf("resp=%v", resp)
	}
	if a.RemoteToolCount() != 3 {
		t.Fatalf("tools=%d", a.RemoteToolCount())
	}
	if names := a.RemoteToolNames(); names != "list_tickets, get_ticket, health" {
		t.Fatalf("names=%q", names)
	}
}

func TestApplyRemoteConfigAllowWrites(t *testing.T) {
	a := newTestAgent()
	body, _ := json.Marshal(map[string]any{
		"api_base": "https://new.example/v1",
		"model":    "new-model",
		"readonly_apis": []map[string]any{
			{
				"id":           "tickets",
				"base_url":     "https://tickets.example.com",
				"spec":         writeSpecJSON,
				"allow_writes": true,
			},
		},
	})
	raw := a.ApplyRemoteConfigJSON(string(body))
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" || resp["readonly_tools"] != float64(5) || resp["write_tools"] != float64(4) {
		t.Fatalf("resp=%v", resp)
	}
	if a.RemoteWriteToolCount() != 4 {
		t.Fatalf("writes=%d", a.RemoteWriteToolCount())
	}
	if names := a.RemoteToolNames(); names != "bulk_create, list_tickets, create_ticket, replace_ticket, patch_ticket" {
		t.Fatalf("names=%q", names)
	}
}

func TestApplyRemoteConfigMethods(t *testing.T) {
	a := newTestAgent()
	body, _ := json.Marshal(map[string]any{
		"api_base": "https://new.example/v1",
		"model":    "new-model",
		"readonly_apis": []map[string]any{
			{
				"id":       "tickets",
				"base_url": "https://tickets.example.com",
				"spec":     writeSpecJSON,
				"methods":  []string{"GET", "PATCH"},
			},
		},
	})
	raw := a.ApplyRemoteConfigJSON(string(body))
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" || resp["readonly_tools"] != float64(2) || resp["write_tools"] != float64(1) {
		t.Fatalf("resp=%v", resp)
	}
	if names := a.RemoteToolNames(); names != "list_tickets, patch_ticket" {
		t.Fatalf("names=%q", names)
	}
}

func TestApplyRemoteConfigReadonlyAPIsMutualExclusion(t *testing.T) {
	a := newTestAgent()
	body, _ := json.Marshal(map[string]any{
		"api_base":     "https://new.example/v1",
		"model":        "m",
		"readonly_api": map[string]any{"base_url": "https://a.example.com", "spec": readonlySpecJSON},
		"readonly_apis": []map[string]any{
			{"base_url": "https://b.example.com", "spec": readonlyHealthSpecJSON},
		},
	})
	raw := a.ApplyRemoteConfigJSON(string(body))
	if !strings.Contains(raw, "mutually") {
		t.Fatalf("raw=%s", raw)
	}
}

func TestApplyRemoteConfigReadonlyAPIsSpecURLRejected(t *testing.T) {
	a := newTestAgent()
	body, _ := json.Marshal(map[string]any{
		"api_base": "https://new.example/v1",
		"model":    "m",
		"readonly_apis": []map[string]any{
			{"spec_url": "https://a.example.com/openapi.json"},
		},
	})
	raw := a.ApplyRemoteConfigJSON(string(body))
	if !strings.Contains(raw, "spec_url") {
		t.Fatalf("raw=%s", raw)
	}
}

func TestApplyRemoteConfigReadonlySpecURLRejected(t *testing.T) {
	a := newTestAgent()
	_ = a.ConfigureReadOnlyAPI("https://api.example.com", "", "", readonlySpecJSON)
	body, _ := json.Marshal(map[string]any{
		"api_base": "https://new.example/v1",
		"model":    "m",
		"readonly_api": map[string]any{
			"spec_url": "https://api.example.com/openapi.json",
		},
	})
	raw := a.ApplyRemoteConfigJSON(string(body))
	if !strings.Contains(raw, "spec_url") {
		t.Fatalf("raw=%s", raw)
	}
	if a.RemoteToolCount() != 2 {
		t.Fatalf("old tools must remain, got %d", a.RemoteToolCount())
	}
}

func TestApplyRemoteConfigReadonlyMutualExclusion(t *testing.T) {
	a := newTestAgent()
	body, _ := json.Marshal(map[string]any{
		"readonly_api": map[string]any{
			"base_url": "https://api.example.com",
			"spec":     readonlySpecJSON,
			"spec_url": "https://api.example.com/openapi.json",
		},
	})
	raw := a.ApplyRemoteConfigJSON(string(body))
	if !strings.Contains(raw, "mutually") {
		t.Fatalf("raw=%s", raw)
	}
}

func TestFetchRemoteConfigReadonlySpecURL(t *testing.T) {
	specSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readonlySpecJSON))
	}))
	t.Cleanup(specSrv.Close)
	configSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"api_base": "https://cfg.example/v1",
			"model":    "cfg-model",
			"readonly_api": map[string]any{
				"base_url":    "https://api.example.com",
				"auth_header": "Authorization",
				"auth_value":  "Bearer secret-token",
				"spec_url":    specSrv.URL,
			},
		})
		_, _ = w.Write(body)
	}))
	t.Cleanup(configSrv.Close)

	a := newTestAgent()
	raw := a.FetchRemoteConfig(configSrv.URL)
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" || resp["readonly_tools"] != float64(2) {
		t.Fatalf("resp=%v", resp)
	}
	if a.RemoteToolCount() != 2 {
		t.Fatalf("tools=%d", a.RemoteToolCount())
	}
	if strings.Contains(raw, "secret-token") || strings.Contains(raw, "openapi") {
		t.Fatalf("response leaked config: %s", raw)
	}
}

func TestFetchRemoteConfigReadonlyAPIsSpecURLs(t *testing.T) {
	ticketsSpec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readonlySpecJSON))
	}))
	t.Cleanup(ticketsSpec.Close)
	healthSpec := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(readonlyHealthSpecJSON))
	}))
	t.Cleanup(healthSpec.Close)
	configSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := json.Marshal(map[string]any{
			"api_base": "https://cfg.example/v1",
			"model":    "cfg-model",
			"readonly_apis": []map[string]any{
				{"id": "tickets", "base_url": "https://tickets.example.com", "spec_url": ticketsSpec.URL},
				{"id": "health", "base_url": "https://health.example.com", "spec_url": healthSpec.URL},
			},
		})
		_, _ = w.Write(body)
	}))
	t.Cleanup(configSrv.Close)

	a := newTestAgent()
	raw := a.FetchRemoteConfig(configSrv.URL)
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "done" || resp["readonly_tools"] != float64(3) || resp["readonly_connectors"] != float64(2) {
		t.Fatalf("resp=%v", resp)
	}
	if strings.Contains(raw, "openapi") || strings.Contains(raw, "spec_url") {
		t.Fatalf("response leaked specs: %s", raw)
	}
}

func TestGoDoesNotPersistAPIKeyToDisk(t *testing.T) {
	dir := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	a := newTestAgent()
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
