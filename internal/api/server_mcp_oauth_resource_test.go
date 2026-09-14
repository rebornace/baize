package api

import (
	"net/url"
	"strings"
	"testing"
)

func TestOAuthResourceURLStripsQueryUserinfo(t *testing.T) {
	got := oauthResourceURL("https://user:pass@mcp.example/v1/mcp?token=secret#frag")
	want := "https://mcp.example/v1/mcp"
	if got != want {
		t.Fatalf("oauthResourceURL=%q want %q", got, want)
	}
}

func TestBuildAuthorizationURLResourceIsOriginPath(t *testing.T) {
	authURL, err := buildAuthorizationURL(
		"https://as.example/authorize",
		"cid",
		"https://app/cb",
		"challenge",
		"state-1",
		"https://user:x@mcp.example/tools?x=1",
	)
	if err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatal(err)
	}
	res := u.Query().Get("resource")
	if res != "https://mcp.example/tools" {
		t.Fatalf("resource=%q want origin+path without query/userinfo", res)
	}
	if strings.Contains(res, "user:") || strings.Contains(res, "?") {
		t.Fatalf("resource leaked credentials/query: %q", res)
	}
}
