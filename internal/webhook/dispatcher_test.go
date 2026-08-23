package webhook_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhook"
)

func TestDispatcherPostsEventAndEnd(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Baize-Protocol") != "v0" {
			t.Errorf("protocol header=%q", r.Header.Get("X-Baize-Protocol"))
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	d := webhook.NewDispatcher(st, webhook.Config{URL: srv.URL})
	d.Attach(hub)

	run, err := st.CreateRun(store.CreateRunInput{AgentID: "a1", Input: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AppendEvent(run.ID, store.Event{Type: "run.started"}); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateRun(run.ID, store.StatusSucceeded, "done", ""); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		n := len(bodies)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) < 2 {
		t.Fatalf("got %d webhook bodies, want >=2: %+v", len(bodies), bodies)
	}
	var sawEvent, sawEnd bool
	for _, b := range bodies {
		if _, ok := b["event"]; ok {
			sawEvent = true
			if b["run_id"] != run.ID {
				t.Fatalf("event body=%+v", b)
			}
		}
		if b["ended"] == true {
			sawEnd = true
			if b["status"] != "succeeded" {
				t.Fatalf("end body=%+v", b)
			}
		}
	}
	if !sawEvent || !sawEnd {
		t.Fatalf("bodies=%+v sawEvent=%v sawEnd=%v", bodies, sawEvent, sawEnd)
	}
}

func TestDispatcherPerRunURLOverride(t *testing.T) {
	globalSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("global webhook should not be called")
	}))
	defer globalSrv.Close()

	var got bool
	perRunSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = true
		w.WriteHeader(http.StatusOK)
	}))
	defer perRunSrv.Close()

	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	d := webhook.NewDispatcher(st, webhook.Config{URL: globalSrv.URL})
	d.Attach(hub)

	run, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a1",
		Input:   "hi",
		WebhookConfig: &store.WebhookConfig{URL: perRunSrv.URL},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(run.ID, store.Event{Type: "run.started"})

	deadline := time.Now().Add(2 * time.Second)
	for !got && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if !got {
		t.Fatal("per-run webhook was not called")
	}
}

func TestSendTest(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	d := webhook.NewDispatcher(store.NewMemory(), webhook.Config{URL: srv.URL})
	if err := d.SendTest(t.Context()); err != nil {
		t.Fatal(err)
	}
	ev, ok := body["event"].(map[string]any)
	if !ok || ev["type"] != "webhook.test" {
		t.Fatalf("body=%+v", body)
	}
}
