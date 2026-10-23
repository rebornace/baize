package mobileagent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestChatOnceRoundTrip(t *testing.T) {
	var calls atomic.Int32
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		n := calls.Add(1)
		gotAuth = r.Header.Get("Authorization")
		var body chatRequest
		_ = json.NewDecoder(r.Body).Decode(&body)
		reply := "pong"
		if n == 1 {
			if body.Model != "demo-model" || len(body.Messages) != 1 || body.Messages[0].Content != "ping" {
				t.Errorf("first request %+v", body)
			}
		} else {
			if len(body.Messages) != 3 {
				t.Errorf("second request expected 3 msgs, got %d", len(body.Messages))
			}
			reply = "again"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": reply}},
			},
		})
	}))
	t.Cleanup(srv.Close)

	a := New()
	if err := a.Configure(srv.URL+"/v1", "test-key", "demo-model"); err != nil {
		t.Fatal(err)
	}
	out, err := a.ChatOnce("ping")
	if err != nil {
		t.Fatal(err)
	}
	if out != "pong" {
		t.Fatalf("got %q", out)
	}
	if gotAuth != "Bearer test-key" {
		t.Fatalf("auth %q", gotAuth)
	}
	if a.MessageCount() != 2 {
		t.Fatalf("message count %d", a.MessageCount())
	}

	out, err = a.ChatOnce("next")
	if err != nil {
		t.Fatal(err)
	}
	if out != "again" || calls.Load() != 2 {
		t.Fatalf("out=%q calls=%d", out, calls.Load())
	}
}

func TestConfigureRequired(t *testing.T) {
	a := New()
	if _, err := a.ChatOnce("hi"); err == nil || !strings.Contains(err.Error(), "Configure") {
		t.Fatalf("expected Configure error, got %v", err)
	}
	if err := a.Configure("", "k", "m"); err == nil {
		t.Fatal("expected empty base error")
	}
}

func TestPackageLevelAPI(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	Reset()
	if err := Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	out, err := ChatOnce("hello")
	if err != nil {
		t.Fatal(err)
	}
	if out != "ok" {
		t.Fatalf("got %q", out)
	}
}
