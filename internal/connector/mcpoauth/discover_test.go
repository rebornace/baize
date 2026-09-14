package mcpoauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseWWWAuthenticateResourceMetadata(t *testing.T) {
	hdr := `Bearer realm="mcp", resource_metadata="https://ex/.well-known/oauth-protected-resource"`
	got, err := ParseResourceMetadataURL(hdr)
	if err != nil {
		t.Fatal(err)
	}
	want := "https://ex/.well-known/oauth-protected-resource"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestDiscoverFromPRM(t *testing.T) {
	const (
		authEP  = "https://as.example/authorize"
		tokenEP = "https://as.example/token"
		regEP   = "https://as.example/register"
	)

	var asBase string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              "https://mcp.example/mcp",
				"authorization_servers": []string{asBase},
			})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 asBase,
				"authorization_endpoint": authEP,
				"token_endpoint":         tokenEP,
				"registration_endpoint":  regEP,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	asBase = srv.URL

	mcpURL := srv.URL + "/mcp"
	ep, err := Discover(context.Background(), srv.Client(), mcpURL)
	if err != nil {
		t.Fatal(err)
	}
	if ep.AuthorizationEndpoint != authEP {
		t.Fatalf("authorization_endpoint %q, want %q", ep.AuthorizationEndpoint, authEP)
	}
	if ep.TokenEndpoint != tokenEP {
		t.Fatalf("token_endpoint %q, want %q", ep.TokenEndpoint, tokenEP)
	}
	if ep.RegistrationEndpoint != regEP {
		t.Fatalf("registration_endpoint %q, want %q", ep.RegistrationEndpoint, regEP)
	}
	wantPRM := srv.URL + "/.well-known/oauth-protected-resource"
	if ep.ResourceMetadataURL != wantPRM {
		t.Fatalf("resource_metadata_url %q, want %q", ep.ResourceMetadataURL, wantPRM)
	}
}
