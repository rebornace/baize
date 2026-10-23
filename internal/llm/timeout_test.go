package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/llm"
)

func TestChatStreamIdleTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if f, ok := w.(http.Flusher); ok {
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n")
			f.Flush()
			// Hold the connection open without further chunks so idle fires.
			time.Sleep(500 * time.Millisecond)
			return
		}
		http.Error(w, "no flusher", http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	p.StreamIdleTimeout = 80 * time.Millisecond
	p.MaxRetries = 0

	_, err := p.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected idle timeout error")
	}
	if !strings.Contains(err.Error(), "stream idle timeout") {
		t.Fatalf("want idle timeout, got %v", err)
	}
}

func TestChatMaxRequestTimeoutStillApplies(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"role": "assistant", "content": "late"}},
			},
		})
	}))
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	p.MaxRequestTimeout = 50 * time.Millisecond
	p.MaxRetries = 0

	_, err := p.Chat(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, nil)
	if err == nil {
		t.Fatal("expected max-request timeout")
	}
}
