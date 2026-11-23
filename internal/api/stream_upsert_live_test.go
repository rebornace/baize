package api_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// Upserted content.delta must keep flowing on SSE (same event id), otherwise
// the chat UI only sees the first snapshot and then the final llm.message dump.
func TestRunStreamResendsUpsertedContentDelta(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.Hub = hub

	run, err := st.CreateRun(store.CreateRunInput{AgentID: "a", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v0/runs/"+run.ID+"/stream?after=-1", nil)
	rr := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.Handler().ServeHTTP(rr, req)
	}()

	// Wait until the stream is open (headers flushed).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if rr.Code == http.StatusOK && strings.Contains(rr.Header().Get("Content-Type"), "event-stream") {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	_, err = st.UpsertStreamingDelta(run.ID, store.Event{
		Type: "llm.content.delta",
		Data: map[string]any{"turn": 0, "text": "你"},
	})
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(50 * time.Millisecond)
	_, err = st.UpsertStreamingDelta(run.ID, store.Event{
		Type: "llm.content.delta",
		Data: map[string]any{"turn": 0, "text": "你好世界"},
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline = time.Now().Add(2 * time.Second)
	var body string
	for time.Now().Before(deadline) {
		body = rr.Body.String()
		if strings.Count(body, "llm.content.delta") >= 2 && strings.Contains(body, "你好世界") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done

	if strings.Count(body, "llm.content.delta") < 2 {
		t.Fatalf("want >=2 content.delta SSE frames (upsert resend), body=%q", body)
	}
	if !strings.Contains(body, "你好世界") {
		t.Fatalf("missing updated snapshot, body=%q", body)
	}
	_ = bufio.NewScanner(strings.NewReader(body))
}
