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

func TestPendingStreamLimitAndStats(t *testing.T) {
	srv, st := pendingTestServer(t)
	srv.PendingStreamLimit = 1
	seedWaitingRun(t, srv, st, "c-limit", "alice", "limit", &store.HITLPayload{
		Kind:   store.HITLKindApproval,
		Prompt: "wait",
	})
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req1, err := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/v0/hitl/pending/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req1.Header.Set("Authorization", "Bearer adm-token")
	resp1, err := http.DefaultClient.Do(req1)
	if err != nil {
		t.Fatal(err)
	}
	defer resp1.Body.Close()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if live, _, _ := srv.pendingStreamStats(); live == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if live, _, _ := srv.pendingStreamStats(); live != 1 {
		t.Fatalf("live=%d want 1", live)
	}

	req2, err := http.NewRequest(http.MethodGet, ts.URL+"/v0/hitl/pending/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	req2.Header.Set("Authorization", "Bearer adm-token")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("second stream status=%d want 429", resp2.StatusCode)
	}
	if _, dropped, _ := srv.pendingStreamStats(); dropped != 1 {
		t.Fatalf("dropped=%d want 1", dropped)
	}

	cancel()
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if live, _, _ := srv.pendingStreamStats(); live == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("stream slot was not released")
}

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
