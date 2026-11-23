package mobileagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNewSessionClonesConfigAndTools(t *testing.T) {
	a := newTestAgent()
	a.SetAllowPrivateNetworks(false)
	if err := a.Configure("https://api.example.com/v1", "secret", "model-a"); err != nil {
		t.Fatal(err)
	}
	cfg := fmt.Sprintf(`[{"id":"tickets","base_url":%q,"spec":%q}]`, "https://api.example.com", readonlySpecJSON)
	if err := a.ConfigureReadOnlyAPIs(cfg); err != nil {
		t.Fatal(err)
	}

	b := a.NewSession()
	if b.baseURL != "https://api.example.com/v1" || b.model != "model-a" || b.apiKey != "secret" {
		t.Fatalf("clone config=%q/%q/%q", b.baseURL, b.model, b.apiKey)
	}
	if b.AllowPrivateNetworks() {
		t.Fatal("clone must not enable private networks")
	}
	if b.RemoteToolCount() != a.RemoteToolCount() {
		t.Fatalf("clone tools=%d want %d", b.RemoteToolCount(), a.RemoteToolCount())
	}
	if b.MessageCount() != 0 {
		t.Fatalf("clone history=%d want 0", b.MessageCount())
	}
	if got := b.PendingApproval(); !strings.Contains(got, `"none"`) {
		t.Fatalf("clone pending=%s", got)
	}
}

func TestSessionHistoryIsolation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	}))
	defer srv.Close()

	a := newTestAgent()
	b := a.NewSession()
	if err := a.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if err := b.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if got := a.Send("first"); !strings.Contains(got, `"done"`) {
		t.Fatalf("a send=%s", got)
	}
	if got := b.Send("second"); !strings.Contains(got, `"done"`) {
		t.Fatalf("b send=%s", got)
	}
	if a.MessageCount() != 2 || b.MessageCount() != 2 {
		t.Fatalf("history a=%d b=%d", a.MessageCount(), b.MessageCount())
	}
	a.mu.Lock()
	aText := a.msgs[0].Content
	a.mu.Unlock()
	b.mu.Lock()
	bText := b.msgs[0].Content
	b.mu.Unlock()
	if aText != "first" || bText != "second" {
		t.Fatalf("crossed history a=%q b=%q", aText, bText)
	}
}

func TestSessionPendingIsolation(t *testing.T) {
	var calls atomic.Int32
	approvalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"http_get","arguments":"{\"url\":\"http://example.com\"}"}}]}}]}`)
			return
		}
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"denied"}}]}`)
	}))
	defer approvalSrv.Close()
	plainSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"plain"}}]}`)
	}))
	defer plainSrv.Close()

	a := newTestAgent()
	b := newTestAgent()
	if err := a.Configure(approvalSrv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if err := b.Configure(plainSrv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	pause := a.Send("fetch")
	if !strings.Contains(pause, `"needs_approval"`) {
		t.Fatalf("a pause=%s", pause)
	}
	aPending := a.PendingApproval()
	if !strings.Contains(aPending, `"call_id":"c1"`) {
		t.Fatalf("a pending=%s", aPending)
	}
	if got := b.PendingApproval(); !strings.Contains(got, `"none"`) {
		t.Fatalf("b pending=%s", got)
	}
	if got := b.Send("plain"); !strings.Contains(got, `"done"`) {
		t.Fatalf("b send=%s", got)
	}
	if got := b.PendingApproval(); !strings.Contains(got, `"none"`) {
		t.Fatalf("b pending after send=%s", got)
	}
	if got := a.Deny("c1"); !strings.Contains(got, `"done"`) {
		t.Fatalf("a deny=%s", got)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(a.PendingApproval()), &parsed); err != nil || parsed["status"] != "none" {
		t.Fatalf("a pending after deny=%s", a.PendingApproval())
	}
}
