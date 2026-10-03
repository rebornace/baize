package mcpoauth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Endpoints holds OAuth discovery results for an MCP resource.
type Endpoints struct {
	ResourceMetadataURL   string
	AuthorizationEndpoint string
	TokenEndpoint         string
	RegistrationEndpoint  string
}

type protectedResourceMetadata struct {
	Resource             string   `json:"resource"`
	AuthorizationServers []string `json:"authorization_servers"`
}

type authorizationServerMetadata struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	RegistrationEndpoint  string `json:"registration_endpoint"`
}

// ParseResourceMetadataURL extracts resource_metadata from a WWW-Authenticate header value.
func ParseResourceMetadataURL(wwwAuthenticate string) (string, error) {
	s := strings.TrimSpace(wwwAuthenticate)
	if len(s) >= 7 && strings.EqualFold(s[:7], "bearer ") {
		s = strings.TrimSpace(s[7:])
	}
	params, err := parseWWWAuthenticateParams(s)
	if err != nil {
		return "", err
	}
	raw, ok := params["resource_metadata"]
	if !ok || raw == "" {
		return "", fmt.Errorf("resource_metadata not found in WWW-Authenticate")
	}
	return raw, nil
}

func parseWWWAuthenticateParams(s string) (map[string]string, error) {
	out := make(map[string]string)
	for _, part := range splitAuthParams(s) {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, val, ok := splitAuthParam(part)
		if !ok {
			continue
		}
		out[strings.ToLower(key)] = val
	}
	return out, nil
}

func splitAuthParams(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			inQuote = !inQuote
			cur.WriteByte(c)
			continue
		}
		if c == ',' && !inQuote {
			parts = append(parts, cur.String())
			cur.Reset()
			continue
		}
		cur.WriteByte(c)
	}
	if cur.Len() > 0 {
		parts = append(parts, cur.String())
	}
	return parts
}

func splitAuthParam(part string) (key, val string, ok bool) {
	eq := strings.Index(part, "=")
	if eq < 0 {
		return "", "", false
	}
	key = strings.TrimSpace(part[:eq])
	val = strings.TrimSpace(part[eq+1:])
	if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
		val = val[1 : len(val)-1]
	}
	return key, val, true
}

// Discover resolves PRM and authorization-server metadata for an MCP resource URL.
func Discover(ctx context.Context, client *http.Client, mcpResourceURL string) (Endpoints, error) {
	if client == nil {
		client = http.DefaultClient
	}
	prmURL, err := resolvePRMURL(ctx, client, mcpResourceURL)
	if err != nil {
		return Endpoints{}, err
	}
	prm, err := fetchPRM(ctx, client, prmURL)
	if err != nil {
		return Endpoints{}, err
	}
	if len(prm.AuthorizationServers) == 0 {
		return Endpoints{}, fmt.Errorf("PRM has no authorization_servers")
	}
	asMeta, err := fetchAuthorizationServerMetadata(ctx, client, prm.AuthorizationServers[0])
	if err != nil {
		return Endpoints{}, err
	}
	if asMeta.AuthorizationEndpoint == "" || asMeta.TokenEndpoint == "" {
		return Endpoints{}, fmt.Errorf("authorization server metadata missing required endpoints")
	}
	return Endpoints{
		ResourceMetadataURL:   prmURL,
		AuthorizationEndpoint: asMeta.AuthorizationEndpoint,
		TokenEndpoint:         asMeta.TokenEndpoint,
		RegistrationEndpoint:  asMeta.RegistrationEndpoint,
	}, nil
}

func resolvePRMURL(ctx context.Context, client *http.Client, mcpResourceURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, mcpResourceURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode == http.StatusUnauthorized {
			if hdr := resp.Header.Get("WWW-Authenticate"); hdr != "" {
				if u, err := ParseResourceMetadataURL(hdr); err == nil && u != "" {
					return u, nil
				}
			}
		}
	}
	return wellKnownPRMURL(mcpResourceURL)
}

func wellKnownPRMURL(mcpResourceURL string) (string, error) {
	u, err := url.Parse(mcpResourceURL)
	if err != nil {
		return "", fmt.Errorf("invalid mcp resource URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid mcp resource URL: missing scheme or host")
	}
	u.Path = "/.well-known/oauth-protected-resource"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}

func fetchPRM(ctx context.Context, client *http.Client, prmURL string) (protectedResourceMetadata, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, prmURL, nil)
	if err != nil {
		return protectedResourceMetadata{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return protectedResourceMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return protectedResourceMetadata{}, fmt.Errorf("PRM fetch %s: status %d", prmURL, resp.StatusCode)
	}
	var prm protectedResourceMetadata
	if err := json.NewDecoder(resp.Body).Decode(&prm); err != nil {
		return protectedResourceMetadata{}, fmt.Errorf("decode PRM: %w", err)
	}
	return prm, nil
}

func fetchAuthorizationServerMetadata(ctx context.Context, client *http.Client, authorizationServer string) (authorizationServerMetadata, error) {
	metaURL, err := authorizationServerMetadataURL(authorizationServer)
	if err != nil {
		return authorizationServerMetadata{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metaURL, nil)
	if err != nil {
		return authorizationServerMetadata{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return authorizationServerMetadata{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return authorizationServerMetadata{}, fmt.Errorf("AS metadata fetch %s: status %d", metaURL, resp.StatusCode)
	}
	var meta authorizationServerMetadata
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return authorizationServerMetadata{}, fmt.Errorf("decode AS metadata: %w", err)
	}
	return meta, nil
}

func authorizationServerMetadataURL(authorizationServer string) (string, error) {
	u, err := url.Parse(strings.TrimRight(authorizationServer, "/"))
	if err != nil {
		return "", fmt.Errorf("invalid authorization server URL: %w", err)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid authorization server URL: missing scheme or host")
	}
	path := strings.TrimSuffix(u.Path, "/")
	if path == "" {
		u.Path = "/.well-known/oauth-authorization-server"
	} else {
		u.Path = "/.well-known/oauth-authorization-server" + path
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
