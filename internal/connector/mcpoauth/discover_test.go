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

func TestDiscoverUsesResourceMetadataFrom401(t *testing.T) {
	const (
		authEP  = "https://as.example/authorize"
		tokenEP = "https://as.example/token"
		prmPath = "/custom/oauth-protected-resource"
	)

	var asBase, mcpOrigin string
	var wellKnownPRMHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/mcp":
			prmURL := mcpOrigin + prmPath
			w.Header().Set("WWW-Authenticate", `Bearer realm="mcp", resource_metadata="`+prmURL+`"`)
			w.WriteHeader(http.StatusUnauthorized)
		case prmPath:
			_ = json.NewEncoder(w).Encode(map[string]any{
				"resource":              "https://mcp.example/mcp",
				"authorization_servers": []string{asBase},
			})
		case "/.well-known/oauth-protected-resource":
			wellKnownPRMHits++
			http.NotFound(w, r)
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer":                 asBase,
				"authorization_endpoint": authEP,
				"token_endpoint":         tokenEP,
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	asBase = srv.URL
	mcpOrigin = srv.URL

	mcpURL := srv.URL + "/mcp"
	wantPRM := srv.URL + prmPath
	ep, err := Discover(context.Background(), srv.Client(), mcpURL)
	if err != nil {
		t.Fatal(err)
	}
	if ep.ResourceMetadataURL != wantPRM {
		t.Fatalf("resource_metadata_url %q, want %q (from WWW-Authenticate)", ep.ResourceMetadataURL, wantPRM)
	}
	if wellKnownPRMHits != 0 {
		t.Fatalf("origin well-known PRM was fetched %d times, want 0", wellKnownPRMHits)
	}
	if ep.AuthorizationEndpoint != authEP || ep.TokenEndpoint != tokenEP {
		t.Fatalf("unexpected endpoints: %+v", ep)
	}
}
