package llm_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/rebornace/baize/internal/llm"
)

func TestOpenAIRetries429ThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "ok"}},
			},
		})
	}))
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	p.MaxRetries = 3
	msg, err := p.Chat(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, nil)
	if err != nil {
		t.Fatalf("chat: %v", err)
	}
	if msg.Content != "ok" {
		t.Fatalf("content=%q", msg.Content)
	}
	if hits.Load() != 3 {
		t.Fatalf("hits=%d want 3", hits.Load())
	}
}

func TestOpenAIZeroRetriesNoRetry(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`rate`))
	}))
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	p.MaxRetries = 0
	_, err := p.Chat(context.Background(), []llm.Message{{Role: llm.RoleUser, Content: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if hits.Load() != 1 {
		t.Fatalf("hits=%d want 1", hits.Load())
	}
}
