package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// flushRecorder ensures ResponseController.Flush succeeds during SSE tests.
type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushRecorder) Flush() {
	f.ResponseRecorder.Flush()
}

func TestRunStreamPollsStoreWithoutHubNudge(t *testing.T) {
	prev := ssePollInterval
	ssePollInterval = 50 * time.Millisecond
	t.Cleanup(func() { ssePollInterval = prev })

	// Hub present so the handler enters the live loop, but store is NOT wrapped
	// with eventbus.Notify — AppendEvent will not nudge subscribers.
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	srv := NewServer(mem, tool.NewRegistry(), &gateFakeRunner{store: mem})
	srv.Hub = hub

	run, err := mem.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.AppendEvent(run.ID, store.Event{Type: "run.started"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream?after=-1", nil)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()

	// Wait until initial replay is visible, then append without Hub notify.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if strings.Contains(rr.Body.String(), "run.started") {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("initial replay not received")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := mem.AppendEvent(run.ID, store.Event{Type: "llm.message", Data: map[string]any{"content": "polled"}}); err != nil {
		cancel()
		t.Fatal(err)
	}

	deadline = time.Now().Add(2 * time.Second)
	for {
		if strings.Contains(rr.Body.String(), "llm.message") {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("poll did not deliver store event: %s", rr.Body.String())
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not exit after cancel")
	}
}

// TestRunStreamExternalNudgeTriggersCatchUp: PublishExternal must not write the
// placeholder as SSE; it should catch up real store events instead.
func TestRunStreamExternalNudgeTriggersCatchUp(t *testing.T) {
	prev := ssePollInterval
	ssePollInterval = time.Hour // ensure delivery is from nudge, not poll
	t.Cleanup(func() { ssePollInterval = prev })

	mem := store.NewMemory()
	hub := eventbus.NewHub()
	srv := NewServer(mem, tool.NewRegistry(), &gateFakeRunner{store: mem})
	srv.Hub = hub

	run, err := mem.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.AppendEvent(run.ID, store.Event{Type: "run.started"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream?after=-1", nil)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if strings.Contains(rr.Body.String(), "run.started") {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("initial replay not received")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := mem.AppendEvent(run.ID, store.Event{Type: "llm.message", Data: map[string]any{"content": "from-nudge"}}); err != nil {
		cancel()
		t.Fatal(err)
	}
	hub.PublishExternal(run.ID, 1) // seq == store index of llm.message

	deadline = time.Now().Add(2 * time.Second)
	for {
		body := rr.Body.String()
		if strings.Contains(body, "llm.message") && strings.Contains(body, "from-nudge") {
			if strings.Contains(body, "external.nudge") {
				cancel()
				<-done
				t.Fatalf("placeholder written to SSE: %s", body)
			}
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("nudge did not catch up real event: %s", body)
		}
		time.Sleep(20 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not exit after cancel")
	}
}

// TestRunStreamEndedCatchUpDeliversStoreEvents: when Ended wins the select and
// the Events channel only had nudges (or was empty), catch-up must still flush
// real store events before writing the terminal frame.
func TestRunStreamEndedCatchUpDeliversStoreEvents(t *testing.T) {
	prev := ssePollInterval
	ssePollInterval = time.Hour // delivery must come from Ended catch-up, not poll
	t.Cleanup(func() { ssePollInterval = prev })

	mem := store.NewMemory()
	hub := eventbus.NewHub()
	// Store is NOT Notify-wrapped: AppendEvent stays in store only.
	srv := NewServer(mem, tool.NewRegistry(), &gateFakeRunner{store: mem})
	srv.Hub = hub

	run, err := mem.CreateRun(store.CreateRunInput{AgentID: "a", Input: "i"})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.AppendEvent(run.ID, store.Event{Type: "run.started"}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream?after=-1", nil)
	rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		srv.Handler().ServeHTTP(rr, req)
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if strings.Contains(rr.Body.String(), "run.started") {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("initial replay not received")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := mem.AppendEvent(run.ID, store.Event{Type: "llm.message", Data: map[string]any{"content": "before-end"}}); err != nil {
		cancel()
		t.Fatal(err)
	}
	// Nudge only (no real IndexedEvent on the Hub) + terminal Ended.
	hub.PublishExternal(run.ID, 1)
	hub.PublishEnd(run.ID, store.StatusSucceeded)

	deadline = time.Now().Add(2 * time.Second)
	for {
		body := rr.Body.String()
		if strings.Contains(body, "llm.message") && strings.Contains(body, "before-end") &&
			strings.Contains(body, "event: run.ended") {
			if strings.Contains(body, "external.nudge") {
				cancel()
				<-done
				t.Fatalf("placeholder written to SSE: %s", body)
			}
			break
		}
		if time.Now().After(deadline) {
			cancel()
			<-done
			t.Fatalf("Ended path did not catch up store event: %s", body)
		}
		time.Sleep(20 * time.Millisecond)
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not exit after ended")
	}
}
