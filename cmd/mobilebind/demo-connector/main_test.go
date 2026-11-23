package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	intma "github.com/rebornace/baize/internal/mobileagent"
)

type demoConfig struct {
	APIBase              string `json:"api_base"`
	Model                string `json:"model"`
	AllowPrivateNetworks bool   `json:"allow_private_networks"`
	ReadOnly             struct {
		BaseURL     string `json:"base_url"`
		AuthHeader  string `json:"auth_header"`
		AuthValue   string `json:"auth_value"`
		SpecURL     string `json:"spec_url"`
		AllowWrites bool   `json:"allow_writes"`
	} `json:"readonly_api"`
}

func fetchConfig(t *testing.T, h http.Handler, host string) demoConfig {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "http://placeholder/config.json", nil)
	req.Host = host
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("config status=%d body=%s", rec.Code, rec.Body.String())
	}
	var cfg demoConfig
	if err := json.Unmarshal(rec.Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestConfigReflectsRequestHost(t *testing.T) {
	h := newHandler(handlerOptions{apiKey: "demo-key", llmPort: "18080", model: "mock"})
	cfg := fetchConfig(t, h, "10.0.2.2:18081")
	if cfg.APIBase != "http://10.0.2.2:18080/v1" {
		t.Fatalf("api_base=%q", cfg.APIBase)
	}
	if cfg.Model != "mock" {
		t.Fatalf("model=%q", cfg.Model)
	}
	if cfg.ReadOnly.BaseURL != "http://10.0.2.2:18081/api" {
		t.Fatalf("base_url=%q", cfg.ReadOnly.BaseURL)
	}
	if cfg.ReadOnly.SpecURL != "http://10.0.2.2:18081/openapi.yaml" {
		t.Fatalf("spec_url=%q", cfg.ReadOnly.SpecURL)
	}
	if cfg.ReadOnly.AuthHeader != "X-API-Key" || cfg.ReadOnly.AuthValue != "demo-key" {
		t.Fatalf("auth=%q %q", cfg.ReadOnly.AuthHeader, cfg.ReadOnly.AuthValue)
	}
	if !cfg.ReadOnly.AllowWrites {
		t.Fatal("demo connector must opt into writes")
	}
	if !cfg.AllowPrivateNetworks {
		t.Fatal("demo config must opt into private networks")
	}
}

func TestConfigWithoutAuthAndWithLLMOverride(t *testing.T) {
	h := newHandler(handlerOptions{llmBase: "https://llm.example/v1", model: "demo-model"})
	cfg := fetchConfig(t, h, "127.0.0.1:18081")
	if cfg.APIBase != "https://llm.example/v1" || cfg.Model != "demo-model" {
		t.Fatalf("llm=%q %q", cfg.APIBase, cfg.Model)
	}
	if cfg.ReadOnly.AuthHeader != "" || cfg.ReadOnly.AuthValue != "" {
		t.Fatalf("auth must be omitted: %q %q", cfg.ReadOnly.AuthHeader, cfg.ReadOnly.AuthValue)
	}
}

func TestFetchRemoteConfigThroughSDK(t *testing.T) {
	srv := httptest.NewServer(newHandler(handlerOptions{apiKey: "demo-key", llmPort: "18080", model: "mock"}))
	t.Cleanup(srv.Close)

	a := intma.New()
	a.SetAllowPrivateNetworks(true)
	raw := a.FetchRemoteConfig(srv.URL + "/config.json")
	var resp map[string]any
	if err := json.Unmarshal([]byte(raw), &resp); err != nil {
		t.Fatalf("resp=%s err=%v", raw, err)
	}
	if resp["status"] != "done" {
		t.Fatalf("resp=%s", raw)
	}
	if a.RemoteToolCount() != 3 {
		t.Fatalf("tools=%d", a.RemoteToolCount())
	}
	if a.RemoteWriteToolCount() != 1 {
		t.Fatalf("writes=%d", a.RemoteWriteToolCount())
	}
	if names := a.RemoteToolNames(); names != "list_tickets, create_ticket, get_ticket" {
		t.Fatalf("names=%q", names)
	}
	if resp["write_tools"] != float64(1) {
		t.Fatalf("write_tools=%v", resp["write_tools"])
	}
}

func TestAPIKeyRequired(t *testing.T) {
	srv := httptest.NewServer(newHandler(handlerOptions{apiKey: "demo-key", llmPort: "18080", model: "mock"}))
	t.Cleanup(srv.Close)

	for _, tc := range []struct {
		name string
		key  string
		want int
	}{
		{"missing", "", http.StatusUnauthorized},
		{"wrong", "nope", http.StatusUnauthorized},
		{"right", "demo-key", http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/tickets", nil)
			if tc.key != "" {
				req.Header.Set("X-API-Key", tc.key)
			}
			res, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if res.StatusCode != tc.want {
				b, _ := io.ReadAll(res.Body)
				t.Fatalf("status=%d body=%s", res.StatusCode, b)
			}
		})
	}
}

func TestCreateTicket(t *testing.T) {
	srv := httptest.NewServer(newHandler(handlerOptions{apiKey: "demo-key", llmPort: "18080", model: "mock"}))
	t.Cleanup(srv.Close)

	post := func(body string, key string) (int, map[string]any) {
		req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/tickets", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		if key != "" {
			req.Header.Set("X-API-Key", key)
		}
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}

	if code, out := post(`{"title":"VPN 故障","priority":"high"}`, "demo-key"); code != http.StatusCreated ||
		out["title"] != "VPN 故障" || out["priority"] != "high" || out["id"] == "" {
		t.Fatalf("create=%d %v", code, out)
	}
	if code, out := post(`{"title":"  "}`, "demo-key"); code != http.StatusBadRequest || out["error"] == nil {
		t.Fatalf("empty title=%d %v", code, out)
	}
	if code, _ := post(`{"title":"x"}`, ""); code != http.StatusUnauthorized {
		t.Fatalf("missing key=%d", code)
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/tickets", nil)
	req.Header.Set("X-API-Key", "demo-key")
	res, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var list map[string]any
	_ = json.NewDecoder(res.Body).Decode(&list)
	if list["count"] != float64(4) {
		t.Fatalf("count=%v", list["count"])
	}
}

func TestListFiltersAndGetTicket(t *testing.T) {
	srv := httptest.NewServer(newHandler(handlerOptions{apiKey: "demo-key", llmPort: "18080", model: "mock"}))
	t.Cleanup(srv.Close)

	list := func(query string) map[string]any {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/tickets"+query, nil)
		req.Header.Set("X-API-Key", "demo-key")
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("status=%d", res.StatusCode)
		}
		var out map[string]any
		if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
			t.Fatal(err)
		}
		return out
	}

	all := list("")
	if all["count"] != float64(3) {
		t.Fatalf("all=%v", all)
	}
	open := list("?status=open&limit=1")
	if open["count"] != float64(1) {
		t.Fatalf("open=%v", open)
	}
	tickets := open["tickets"].([]any)
	if got := tickets[0].(map[string]any)["status"]; got != "open" {
		t.Fatalf("status=%v", got)
	}
	closed := list("?status=closed")
	if closed["count"] != float64(1) {
		t.Fatalf("closed=%v", closed)
	}

	get := func(id string) (int, map[string]any) {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/tickets/"+id, nil)
		req.Header.Set("X-API-Key", "demo-key")
		res, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var out map[string]any
		_ = json.NewDecoder(res.Body).Decode(&out)
		return res.StatusCode, out
	}
	if code, out := get("T-1002"); code != http.StatusOK || out["id"] != "T-1002" {
		t.Fatalf("get=%d %v", code, out)
	}
	if code, out := get("missing"); code != http.StatusNotFound || out["error"] == nil {
		t.Fatalf("missing=%d %v", code, out)
	}
}

func TestSpecIsServed(t *testing.T) {
	h := newHandler(handlerOptions{apiKey: "demo-key", llmPort: "18080", model: "mock"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.yaml", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"openapi: 3.0.3", "list_tickets", "create_ticket", "get_ticket", "/tickets/{id}"} {
		if !strings.Contains(body, want) {
			t.Fatalf("spec missing %q", want)
		}
	}
}

func TestHealthz(t *testing.T) {
	h := newHandler(handlerOptions{})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
		t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
	}
}
