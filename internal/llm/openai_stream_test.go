package llm_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/llm"
)

func TestChatStreamThinkingAndContent(t *testing.T) {
	const sse = "" +
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"A\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"B\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n" +
		"data: {\"choices\":[{\"delta\":{\"content\":\"!\"}}]}\n\n" +
		"data: [DONE]\n\n"

	var capturedBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		capturedBody = b
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sse))
	}))
	defer srv.Close()

	p := llm.NewOpenAI(srv.URL, "k", "m")
	var thinks, contents []string
	msg, err := p.ChatStream(context.Background(), []llm.Message{
		{Role: llm.RoleUser, Content: "hi"},
	}, nil, func(s string) {
		thinks = append(thinks, s)
	}, func(s string) {
		contents = append(contents, s)
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	var req map[string]any
	if err := json.Unmarshal(capturedBody, &req); err != nil {
		t.Fatalf("unmarshal request: %v\nbody: %s", err, capturedBody)
	}
	if req["stream"] != true {
		t.Fatalf("want stream:true, body=%s", capturedBody)
	}

	if len(thinks) == 0 || thinks[len(thinks)-1] != "AB" {
		t.Fatalf("onThink last=%v want AB", thinks)
	}
	if len(contents) == 0 || contents[len(contents)-1] != "hi!" {
		t.Fatalf("onContent last=%v want hi!", contents)
	}
	if msg.Content != "hi!" {
		t.Fatalf("Content=%q want hi!", msg.Content)
	}
	if msg.Thinking != "AB" {
		t.Fatalf("Thinking=%q want AB", msg.Thinking)
	}
}

func TestCoalescerRateLimitsThink(t *testing.T) {
	var emits []string
	c := llm.NewCoalescer(50*time.Millisecond, func(s string) {
		emits = append(emits, s)
	}, nil)

	base := time.Unix(0, 0).UTC()
	n := 0
	times := []time.Time{base, base.Add(time.Millisecond), base.Add(2 * time.Millisecond)}
	c.Now = func() time.Time {
		t := times[n]
		if n < len(times)-1 {
			n++
		}
		return t
	}

	c.Think("A")
	c.Think("AB")
	c.Think("ABC")
	if len(emits) != 1 {
		t.Fatalf("want 1 intermediate emit, got %d: %v", len(emits), emits)
	}
	if emits[0] != "A" {
		t.Fatalf("intermediate=%q want A", emits[0])
	}

	c.Flush()
	if len(emits) != 2 {
		t.Fatalf("want Flush to emit final, got %d: %v", len(emits), emits)
	}
	if emits[1] != "ABC" {
		t.Fatalf("final=%q want ABC", emits[1])
	}
}
