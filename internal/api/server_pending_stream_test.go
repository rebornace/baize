package api

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestPendingStreamInitialAndUpdate(t *testing.T) {
	srv, st := pendingTestServer(t)
	seedWaitingRun(t, srv, st, "c-stream", "alice", "stream test", &store.HITLPayload{
		Kind:     store.HITLKindApproval,
		ToolName: "send_email",
		Prompt:   "send?",
	})

	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v0/hitl/pending/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer adm-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("content-type=%q", ct)
	}

	scanner := bufio.NewScanner(resp.Body)
	readEvent := func() map[string]any {
		for scanner.Scan() {
			line := scanner.Text()
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var body map[string]any
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &body); err != nil {
				t.Fatalf("event json: %v", err)
			}
			return body
		}
		t.Fatalf("stream ended: %v", scanner.Err())
		return nil
	}

	first := readEvent()
	if first["count"].(float64) != 1 {
		t.Fatalf("initial=%+v", first)
	}

	seedWaitingRun(t, srv, st, "c-stream-2", "alice", "second", &store.HITLPayload{
		Kind:   store.HITLKindLogin,
		Prompt: "login?",
	})
	second := readEvent()
	if second["count"].(float64) != 2 {
		t.Fatalf("update=%+v", second)
	}
}
