package mcpoauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ClientRegistration is the result of dynamic client registration (RFC 7591).
type ClientRegistration struct {
	ClientID     string
	ClientSecret string
}

// RegisterClient registers a public OAuth client at registrationEndpoint.
// It prefers token_endpoint_auth_method=none (OAuth 2.1 public client / PKCE).
func RegisterClient(ctx context.Context, client *http.Client, registrationEndpoint, redirectURI string) (ClientRegistration, error) {
	if client == nil {
		client = http.DefaultClient
	}
	redirectURI = strings.TrimSpace(redirectURI)
	if registrationEndpoint == "" || redirectURI == "" {
		return ClientRegistration{}, fmt.Errorf("registration_endpoint and redirect_uri are required")
	}
	payload := map[string]any{
		"redirect_uris":              []string{redirectURI},
		"token_endpoint_auth_method": "none",
		"grant_types":                []string{"authorization_code", "refresh_token"},
		"response_types":             []string{"code"},
		"client_name":                "baize",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return ClientRegistration{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registrationEndpoint, bytes.NewReader(raw))
	if err != nil {
		return ClientRegistration{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return ClientRegistration{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ClientRegistration{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ClientRegistration{}, fmt.Errorf("DCR: status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var out struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return ClientRegistration{}, fmt.Errorf("decode DCR response: %w", err)
	}
	if out.ClientID == "" {
		return ClientRegistration{}, fmt.Errorf("DCR response missing client_id")
	}
	return ClientRegistration{ClientID: out.ClientID, ClientSecret: out.ClientSecret}, nil
}
