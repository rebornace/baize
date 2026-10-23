package mobileagent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// remoteConfigJSON is the optional remote settings document.
type remoteConfigJSON struct {
	APIBase string `json:"api_base"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key"`
}

// ApplyRemoteConfigJSON merges api_base / model / optional api_key into memory.
// Does not write secrets to disk. Returns JSON status.
func (a *Agent) ApplyRemoteConfigJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return errJSON("empty config json")
	}
	var cfg remoteConfigJSON
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return errJSON("invalid json: " + err.Error())
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if v := strings.TrimRight(strings.TrimSpace(cfg.APIBase), "/"); v != "" {
		a.baseURL = v
	}
	if v := strings.TrimSpace(cfg.Model); v != "" {
		a.model = v
	}
	if v := strings.TrimSpace(cfg.APIKey); v != "" {
		a.apiKey = v
	}
	if a.baseURL == "" || a.model == "" {
		return errJSON("api_base and model must be set (via Configure or JSON)")
	}
	b, _ := json.Marshal(map[string]any{
		"status":   "done",
		"api_base": a.baseURL,
		"model":    a.model,
		"has_key":  a.apiKey != "",
	})
	return string(b)
}

// ApplyRemoteConfigJSON on the default session.
func ApplyRemoteConfigJSON(raw string) string {
	return defaultSession.ApplyRemoteConfigJSON(raw)
}

// FetchRemoteConfig GETs a JSON config document from url (http/https).
func (a *Agent) FetchRemoteConfig(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return errJSON("url is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return errJSON(err.Error())
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errJSON("only http/https allowed")
	}
	client := a.fetchClient
	if client == nil {
		client = &http.Client{Timeout: httpGetTimeout}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return errJSON(err.Error())
	}
	resp, err := client.Do(req)
	if err != nil {
		return errJSON(fmt.Sprintf("fetch: %v", err))
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil {
		return errJSON(err.Error())
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errJSON(fmt.Sprintf("HTTP %d: %s", resp.StatusCode, truncate(string(body), 120)))
	}
	// Validate shape then return applied merge + raw echo.
	applied := a.ApplyRemoteConfigJSON(string(body))
	var wrap map[string]any
	if err := json.Unmarshal([]byte(applied), &wrap); err != nil {
		return applied
	}
	wrap["fetched"] = true
	out, _ := json.Marshal(wrap)
	return string(out)
}

// FetchRemoteConfig on the default session.
func FetchRemoteConfig(rawURL string) string {
	return defaultSession.FetchRemoteConfig(rawURL)
}

// HasAPIKey reports whether an API key is present in memory (not persisted by Go).
func (a *Agent) HasAPIKey() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.apiKey != ""
}

// HasAPIKey for the default session.
func HasAPIKey() bool { return defaultSession.HasAPIKey() }
