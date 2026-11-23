package mobileagent

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsPublicIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1",
		"::1",
		"10.1.2.3",
		"172.16.0.1",
		"192.168.1.10",
		"169.254.169.254",
		"100.64.0.1",
		"0.0.0.0",
		"::",
		"fc00::1",
		"fe80::1",
		"fec0::1",
		"224.0.0.1",
		"255.255.255.255",
		"::ffff:127.0.0.1",
	}
	for _, raw := range blocked {
		if ip := net.ParseIP(raw); ip == nil || isPublicIP(ip) {
			t.Errorf("isPublicIP(%s)=true, want blocked", raw)
		}
	}
	allowed := []string{"1.1.1.1", "8.8.8.8", "93.184.216.34", "2606:4700:4700::1111"}
	for _, raw := range allowed {
		if ip := net.ParseIP(raw); ip == nil || !isPublicIP(ip) {
			t.Errorf("isPublicIP(%s)=false, want allowed", raw)
		}
	}
}

func TestHTTPGetBlocksPrivateByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	a := New()
	_, err := a.execHTTPGet(a.httpGetClient(), fmt.Sprintf(`{"url":%q}`, srv.URL))
	if err == nil || !errors.Is(err, errBlockedNetwork) {
		t.Fatalf("private url err=%v, want blocked sentinel", err)
	}
	if strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("blocked error leaked address: %v", err)
	}

	a.SetAllowPrivateNetworks(true)
	if !a.AllowPrivateNetworks() {
		t.Fatal("AllowPrivateNetworks()=false after enable")
	}
	out, err := a.execHTTPGet(a.httpGetClient(), fmt.Sprintf(`{"url":%q}`, srv.URL))
	if err != nil {
		t.Fatalf("allowed private url: %v", err)
	}
	if !strings.Contains(out, `"status":200`) || !strings.Contains(out, "ok") {
		t.Fatalf("unexpected result: %s", out)
	}
}

func TestHTTPGetBlocksLocalhostHostname(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	}))
	defer srv.Close()
	u := strings.Replace(srv.URL, "127.0.0.1", "localhost", 1)

	a := New()
	_, err := a.execHTTPGet(a.httpGetClient(), fmt.Sprintf(`{"url":%q}`, u))
	if err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("localhost err=%v, want blocked", err)
	}
}

func TestConnectorPrivateNetworkHonorsSessionPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	const spec = `openapi: 3.0.3
info: {title: t, version: "1"}
paths:
  /ping:
    get:
      operationId: ping
      responses: {"200": {description: ok}}
`
	a := New()
	cfg := fmt.Sprintf(`[{"id":"local","base_url":%q,"spec":%q}]`, srv.URL, spec)
	if err := a.ConfigureReadOnlyAPIs(cfg); err != nil {
		t.Fatal(err)
	}
	a.mu.Lock()
	tool := a.remoteToolLocked("ping")
	a.mu.Unlock()
	if tool == nil {
		t.Fatal("ping tool not registered")
	}
	if _, err := a.execRemoteTool(tool, "{}"); err == nil || !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("connector before opt-in err=%v, want blocked", err)
	}

	a.SetAllowPrivateNetworks(true)
	out, err := a.execRemoteTool(tool, "{}")
	if err != nil || !strings.Contains(out, "ok") {
		t.Fatalf("connector after opt-in out=%s err=%v", out, err)
	}
}

func TestFetchRemoteConfigHonorsPrivatePolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"api_base":"https://api.example.com","model":"m"}`)
	}))
	defer srv.Close()

	a := New()
	if got := a.FetchRemoteConfig(srv.URL); !strings.Contains(got, "not allowed") {
		t.Fatalf("remote config got=%s, want blocked", got)
	} else if strings.Contains(got, "127.0.0.1") {
		t.Fatalf("blocked error leaked address: %s", got)
	}
	a.SetAllowPrivateNetworks(true)
	if got := a.FetchRemoteConfig(srv.URL); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("remote config after enable=%s", got)
	}
}

func TestRemoteConfigPrivatePolicyMerges(t *testing.T) {
	a := New()
	raw := a.ApplyRemoteConfigJSON(`{"api_base":"https://api.example.com","model":"m","allow_private_networks":true}`)
	if !strings.Contains(raw, `"allow_private_networks":true`) || !a.AllowPrivateNetworks() {
		t.Fatalf("explicit enable not applied: %s", raw)
	}
	// Omitting the field must not silently reset the policy.
	raw = a.ApplyRemoteConfigJSON(`{"api_base":"https://api.example.com","model":"m"}`)
	if !a.AllowPrivateNetworks() || !strings.Contains(raw, `"allow_private_networks":true`) {
		t.Fatalf("omitted field reset policy: %s", raw)
	}
	raw = a.ApplyRemoteConfigJSON(`{"api_base":"https://api.example.com","model":"m","allow_private_networks":false}`)
	if a.AllowPrivateNetworks() || !strings.Contains(raw, `"allow_private_networks":false`) {
		t.Fatalf("explicit disable not applied: %s", raw)
	}
}

func TestPrivateAllowlistCIDR(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	a := New()
	got := a.SetPrivateNetworkAllowlist(`{"cidrs":["127.0.0.1/32"]}`)
	if !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("allowlist=%s", got)
	}
	out, err := a.execHTTPGet(a.httpGetClient(), fmt.Sprintf(`{"url":%q}`, srv.URL))
	if err != nil || !strings.Contains(out, "ok") {
		t.Fatalf("allowlisted cidr out=%s err=%v", out, err)
	}
	// A different loopback address is outside the /32 and must be blocked by
	// the dialer before any connection attempt.
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))
	_, err = a.execHTTPGet(a.httpGetClient(), fmt.Sprintf(`{"url":"http://127.0.0.2:%s/"}`, port))
	if err == nil || !errors.Is(err, errBlockedNetwork) {
		t.Fatalf("outside cidr err=%v", err)
	}
}

func TestPrivateAllowlistHostRestriction(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "http://"))

	a := New()
	got := a.SetPrivateNetworkAllowlist(`{"hosts":["localhost"],"cidrs":["127.0.0.1/32"]}`)
	if !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("allowlist=%s", got)
	}
	out, err := a.execHTTPGet(a.httpGetClient(), fmt.Sprintf(`{"url":"http://localhost:%s/"}`, port))
	if err != nil || !strings.Contains(out, "ok") {
		t.Fatalf("allowed host out=%s err=%v", out, err)
	}

	b := New()
	if got := b.SetPrivateNetworkAllowlist(`{"hosts":["other.local"],"cidrs":["127.0.0.1/32"]}`); !strings.Contains(got, `"status":"done"`) {
		t.Fatalf("allowlist=%s", got)
	}
	if _, err := b.execHTTPGet(b.httpGetClient(), fmt.Sprintf(`{"url":"http://localhost:%s/"}`, port)); !errors.Is(err, errBlockedNetwork) {
		t.Fatalf("host not listed err=%v", err)
	}
	// IP literals are governed by the CIDR, not the host list.
	if out, err := b.execHTTPGet(b.httpGetClient(), fmt.Sprintf(`{"url":%q}`, srv.URL)); err != nil || !strings.Contains(out, "ok") {
		t.Fatalf("ip literal out=%s err=%v", out, err)
	}
}

func TestPrivateAllowlistValidation(t *testing.T) {
	a := New()
	for _, raw := range []string{
		`{"cidrs":["nope"]}`,
		`{"hosts":["10.0.0.1"],"cidrs":["10.0.0.0/8"]}`,
	} {
		if got := a.SetPrivateNetworkAllowlist(raw); !strings.Contains(got, `"status":"error"`) {
			t.Fatalf("%s => %s, want error", raw, got)
		}
	}
}

func TestRemoteConfigPrivateAllowlist(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"ok":true}`)
	}))
	defer srv.Close()

	a := New()
	raw := a.ApplyRemoteConfigJSON(`{"api_base":"https://api.example.com","model":"m","allowed_private_cidrs":["127.0.0.1/32"]}`)
	if !strings.Contains(raw, `"status":"done"`) {
		t.Fatalf("config=%s", raw)
	}
	out, err := a.execHTTPGet(a.httpGetClient(), fmt.Sprintf(`{"url":%q}`, srv.URL))
	if err != nil || !strings.Contains(out, "ok") {
		t.Fatalf("config allowlist out=%s err=%v", out, err)
	}
}
