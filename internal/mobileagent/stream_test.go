package mobileagent

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func sseChunk(content string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"delta": map[string]any{"content": content}}},
	})
	return "data: " + string(b) + "\n\n"
}

func sseToolCall(id, name, args string) string {
	b, _ := json.Marshal(map[string]any{
		"choices": []map[string]any{{"delta": map[string]any{
			"tool_calls": []map[string]any{{
				"index": 0, "id": id, "type": "function",
				"function": map[string]any{"name": name, "arguments": args},
			}},
		}}},
	})
	return "data: " + string(b) + "\n\n"
}

func waitStream(t *testing.T, a *Agent) map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	var last map[string]any
	for time.Now().Before(deadline) {
		raw := a.PollStream()
		var got map[string]any
		if err := json.Unmarshal([]byte(raw), &got); err != nil {
			t.Fatalf("poll json=%s err=%v", raw, err)
		}
		last = got
		if got["final"] == true {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("stream did not finish: %+v", last)
	return nil
}

func TestStreamMatchesNonStreamText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if !req.Stream {
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"Hello"}}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, sseChunk("Hel"))
		flusher.Flush()
		fmt.Fprint(w, sseChunk("lo"))
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	plain := newTestAgent()
	if err := plain.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	done := plain.Send("hi")
	if !strings.Contains(done, `"text":"Hello"`) {
		t.Fatalf("plain done=%s", done)
	}

	streamed := newTestAgent()
	if err := streamed.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if got := streamed.StartStream("hi"); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("start=%s", got)
	}
	last := waitStream(t, streamed)
	if last["status"] != "done" || last["text"] != "Hello" {
		t.Fatalf("stream final=%+v", last)
	}
}

func TestStreamFallsBackToPlainJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"plain"}}]}`)
	}))
	defer srv.Close()

	a := newTestAgent()
	if err := a.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if got := a.StartStream("hi"); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("start=%s", got)
	}
	last := waitStream(t, a)
	if last["status"] != "done" || last["text"] != "plain" {
		t.Fatalf("fallback final=%+v", last)
	}
}

func TestStreamEndsAtApproval(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Stream bool `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !req.Stream {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"denied"}}]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseToolCall("c1", "http_get", `{"url":"http://example.com"}`))
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := newTestAgent()
	if err := a.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if got := a.StartStream("fetch"); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("start=%s", got)
	}
	last := waitStream(t, a)
	if last["status"] != "needs_approval" {
		t.Fatalf("want approval pause, got %+v", last)
	}
	if !strings.Contains(fmt.Sprint(last["result"]), `"call_id":"c1"`) {
		t.Fatalf("pause result=%v", last["result"])
	}
	// Approve/Deny still work after the stream ended.
	if got := a.Deny("c1"); !strings.Contains(got, `"done"`) {
		t.Fatalf("deny=%s", got)
	}
}

func TestStreamErrorKeepsPartialTextAndRetryReusesTurn(t *testing.T) {
	var calls int32
	var lastUserCount int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []chatMessage `json:"messages"`
			Stream   bool          `json:"stream"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if req.Stream {
			users := 0
			for _, m := range req.Messages {
				if m.Role == "user" {
					users++
				}
			}
			atomic.StoreInt32(&lastUserCount, int32(users))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		if atomic.AddInt32(&calls, 1) == 1 {
			fmt.Fprint(w, sseChunk("partial"))
			flusher.Flush()
			fmt.Fprint(w, "data: {not-json}\n\n")
			flusher.Flush()
			return
		}
		fmt.Fprint(w, sseChunk("ok"))
		flusher.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	a := newTestAgent()
	if err := a.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if got := a.StartStream("hi"); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("start=%s", got)
	}
	failed := waitStream(t, a)
	if failed["status"] != "error" || failed["text"] != "partial" {
		t.Fatalf("failed stream=%+v", failed)
	}
	if !strings.Contains(fmt.Sprint(failed["result"]), "decode stream") {
		t.Fatalf("result=%v", failed["result"])
	}
	if a.MessageCount() != 1 {
		t.Fatalf("message count after error=%d want 1", a.MessageCount())
	}

	if got := a.RetryStream(); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("retry=%s", got)
	}
	done := waitStream(t, a)
	if done["status"] != "done" || done["text"] != "ok" {
		t.Fatalf("retry final=%+v", done)
	}
	if got := atomic.LoadInt32(&lastUserCount); got != 1 {
		t.Fatalf("retry must send one user turn, got %d", got)
	}
	if a.MessageCount() != 2 {
		t.Fatalf("message count after retry=%d want user+assistant", a.MessageCount())
	}
	if got := a.RetryStream(); !strings.Contains(got, "nothing to retry") {
		t.Fatalf("retry after done=%s", got)
	}
}

func TestStreamCancel(t *testing.T) {
	started := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		fmt.Fprint(w, sseChunk("partial"))
		flusher.Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer srv.Close()

	a := newTestAgent()
	if err := a.Configure(srv.URL, "k", "m"); err != nil {
		t.Fatal(err)
	}
	if got := a.StartStream("hi"); !strings.Contains(got, `"streaming"`) {
		t.Fatalf("start=%s", got)
	}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not start")
	}
	// Wait until the client has consumed the first chunk; cancellation can
	// otherwise race the scanner and leave the accumulated text empty.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var got map[string]any
		if err := json.Unmarshal([]byte(a.PollStream()), &got); err == nil && got["text"] == "partial" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := a.CancelStream(); !strings.Contains(got, "cancelling") {
		t.Fatalf("cancel=%s", got)
	}
	last := waitStream(t, a)
	if last["status"] != "cancelled" {
		t.Fatalf("cancel final=%+v", last)
	}
	if last["text"] != "partial" {
		t.Fatalf("cancelled text=%v", last["text"])
	}
}
