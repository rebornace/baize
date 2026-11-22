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
	APIBase      string                    `json:"api_base"`
	Model        string                    `json:"model"`
	APIKey       string                    `json:"api_key"`
	ReadOnlyAPI  *readOnlyAPIRemoteConfig  `json:"readonly_api"`
	ReadOnlyAPIs []readOnlyAPIRemoteConfig `json:"readonly_apis"`
}

type readOnlyAPIRemoteConfig struct {
	ID          string   `json:"id"`
	BaseURL     string   `json:"base_url"`
	AuthHeader  string   `json:"auth_header"`
	AuthValue   string   `json:"auth_value"`
	Spec        string   `json:"spec"`
	SpecURL     string   `json:"spec_url"`
	AllowWrites bool     `json:"allow_writes"`
	Methods     []string `json:"methods"`
}

// connectorConfig converts one remote entry into the parser input, rejecting
// unresolved spec_url (only FetchRemoteConfig may download it).
func (c readOnlyAPIRemoteConfig) connectorConfig() (readOnlyConnectorConfig, error) {
	spec := strings.TrimSpace(c.Spec)
	specURL := strings.TrimSpace(c.SpecURL)
	if spec != "" && specURL != "" {
		return readOnlyConnectorConfig{}, fmt.Errorf("spec and spec_url are mutually exclusive")
	}
	if specURL != "" {
		return readOnlyConnectorConfig{}, fmt.Errorf("spec_url must be resolved by FetchRemoteConfig")
	}
	if spec == "" {
		return readOnlyConnectorConfig{}, fmt.Errorf("spec is required")
	}
	return readOnlyConnectorConfig{
		ID:          strings.TrimSpace(c.ID),
		BaseURL:     c.BaseURL,
		AuthHeader:  c.AuthHeader,
		AuthValue:   c.AuthValue,
		Spec:        spec,
		AllowWrites: c.AllowWrites,
		Methods:     c.Methods,
	}, nil
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
	if cfg.ReadOnlyAPI != nil && len(cfg.ReadOnlyAPIs) > 0 {
		return errJSON("readonly_api and readonly_apis are mutually exclusive")
	}
	if cfg.ReadOnlyAPIs != nil && len(cfg.ReadOnlyAPIs) == 0 {
		return errJSON("readonly_apis: at least one connector is required")
	}
	var readonlyConfigs []readOnlyConnectorConfig
	if cfg.ReadOnlyAPI != nil {
		cc, err := cfg.ReadOnlyAPI.connectorConfig()
		if err != nil {
			return errJSON("readonly_api: " + err.Error())
		}
		readonlyConfigs = append(readonlyConfigs, cc)
	} else if len(cfg.ReadOnlyAPIs) > 0 {
		for i, rc := range cfg.ReadOnlyAPIs {
			cc, err := rc.connectorConfig()
			if err != nil {
				return errJSON(fmt.Sprintf("readonly_apis[%d]: %v", i, err))
			}
			readonlyConfigs = append(readonlyConfigs, cc)
		}
	}
	var readonly *readonlyAPI
	if len(readonlyConfigs) > 0 {
		parsed, err := parseReadOnlyAPIs(readonlyConfigs)
		if err != nil {
			return errJSON(err.Error())
		}
		readonly = parsed
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
	if readonly != nil {
		a.readOnly = readonly
		if a.pending != nil && a.pending.Remote != nil {
			a.pending = nil
		}
	}
	count, connectors, writes := 0, 0, 0
	if a.readOnly != nil {
		count = len(a.readOnly.tools)
		connectors = a.readOnly.connectors
		writes = writeToolCount(a.readOnly)
	}
	b, _ := json.Marshal(map[string]any{
		"status":              "done",
		"api_base":            a.baseURL,
		"model":               a.model,
		"has_key":             a.apiKey != "",
		"readonly_tools":      count,
		"readonly_connectors": connectors,
		"write_tools":         writes,
	})
	return string(b)
}

// ApplyRemoteConfigJSON on the default session.
func ApplyRemoteConfigJSON(raw string) string {
	return defaultSession.ApplyRemoteConfigJSON(raw)
}

func (a *Agent) resolveReadOnlySpecURL(client *http.Client, body []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("invalid json: %w", err)
	}
	_, hasSingle := doc["readonly_api"]
	_, hasMany := doc["readonly_apis"]
	if hasSingle && hasMany {
		return nil, fmt.Errorf("readonly_api and readonly_apis are mutually exclusive")
	}
	if hasSingle {
		raw, ok := doc["readonly_api"].(map[string]any)
		if !ok {
			return body, nil
		}
		if err := resolveReadOnlySpecURLIn(client, raw); err != nil {
			return nil, err
		}
	}
	if hasMany {
		list, ok := doc["readonly_apis"].([]any)
		if !ok {
			return body, nil
		}
		for i, item := range list {
			raw, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if err := resolveReadOnlySpecURLIn(client, raw); err != nil {
				return nil, fmt.Errorf("readonly_apis[%d]: %w", i, err)
			}
		}
	}
	return json.Marshal(doc)
}

func resolveReadOnlySpecURLIn(client *http.Client, raw map[string]any) error {
	spec := mapString(raw, "spec")
	specURL := mapString(raw, "spec_url")
	if spec != "" && specURL != "" {
		return fmt.Errorf("spec and spec_url are mutually exclusive")
	}
	if specURL == "" {
		return nil
	}
	fetched, err := fetchReadOnlySpec(client, specURL)
	if err != nil {
		return err
	}
	raw["spec"] = string(fetched)
	delete(raw, "spec_url")
	return nil
}

func fetchReadOnlySpec(client *http.Client, rawURL string) ([]byte, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("readonly_api: spec_url must be absolute http(s)")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("readonly_api: spec_url must use http or https")
	}
	if client == nil {
		client = &http.Client{Timeout: httpGetTimeout}
	}
	ctx, cancel := context.WithTimeout(context.Background(), httpGetTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("readonly_api: fetch spec: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, httpGetMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("readonly_api: spec HTTP %d", resp.StatusCode)
	}
	if len(raw) > httpGetMaxBytes {
		return nil, fmt.Errorf("readonly_api: spec exceeds %d bytes", httpGetMaxBytes)
	}
	return raw, nil
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
	configBody, err := a.resolveReadOnlySpecURL(client, body)
	if err != nil {
		return errJSON(err.Error())
	}
	// Validate shape then return applied merge + raw echo.
	applied := a.ApplyRemoteConfigJSON(string(configBody))
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
