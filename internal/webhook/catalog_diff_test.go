package webhook_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/connector"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/webhook"
)

func TestNotifyCatalogDiffPostsPayload(t *testing.T) {
	var mu sync.Mutex
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Baize-Run-Id") != "connector:conn1" {
			t.Errorf("run id header=%q", r.Header.Get("X-Baize-Run-Id"))
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		got = body
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	mem := store.NewMemory()
	d := webhook.NewDispatcher(mem, webhook.Config{URL: srv.URL})
	diff := connector.CatalogDiff{
		Added:   []string{"a"},
		Removed: []string{"b"},
		Changed: []connector.CatalogChange{{Name: "c", SchemaChanged: true}},
	}
	d.NotifyCatalogDiff("conn1", 3, diff)

	due, err := mem.ListWebhookOutboxDue(time.Now().Add(time.Hour), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("due=%v err=%v", due, err)
	}
	entry := due[0]
	if entry.RunID != "connector:conn1" {
		t.Fatalf("run_id=%q", entry.RunID)
	}
	if entry.Kind != store.WebhookOutboxKindCatalogDiff {
		t.Fatalf("kind=%q", entry.Kind)
	}
	if entry.EventIndex != 3 {
		t.Fatalf("event_index=%d", entry.EventIndex)
	}

	var payload map[string]any
	if err := json.Unmarshal(entry.PayloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["type"] != "catalog.diff" {
		t.Fatalf("payload=%+v", payload)
	}
	if payload["connector_id"] != "conn1" {
		t.Fatalf("connector_id=%v", payload["connector_id"])
	}
	if payload["catalog_revision"] != float64(3) {
		t.Fatalf("catalog_revision=%v", payload["catalog_revision"])
	}
	diffMap, ok := payload["diff"].(map[string]any)
	if !ok {
		t.Fatalf("diff=%v", payload["diff"])
	}
	added, _ := diffMap["added"].([]any)
	if len(added) != 1 || added[0] != "a" {
		t.Fatalf("added=%v", diffMap["added"])
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.StartWorker(ctx)

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		ready := got != nil && got["type"] == "catalog.diff"
		mu.Unlock()
		if ready || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	mu.Lock()
	defer mu.Unlock()
	if got == nil || got["type"] != "catalog.diff" {
		t.Fatalf("posted body=%+v", got)
	}
}

func TestNotifyCatalogDiffEmptySkipped(t *testing.T) {
	st := store.NewMemory()
	d := webhook.NewDispatcher(st, webhook.Config{URL: "http://127.0.0.1:9"})
	d.NotifyCatalogDiff("c", 1, connector.CatalogDiff{})
	due, err := st.ListWebhookOutboxDue(time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("want no outbox, got %d", len(due))
	}
}

func TestNotifyCatalogDiffNoURLSkipped(t *testing.T) {
	st := store.NewMemory()
	d := webhook.NewDispatcher(st, webhook.Config{})
	d.NotifyCatalogDiff("c", 2, connector.CatalogDiff{Added: []string{"x"}})
	due, err := st.ListWebhookOutboxDue(time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatal("want skip when no URL")
	}
}

func TestNotifyCatalogDiffDedupesRevision(t *testing.T) {
	st := store.NewMemory()
	d := webhook.NewDispatcher(st, webhook.Config{URL: "http://example.invalid"})
	diff := connector.CatalogDiff{Added: []string{"x"}}
	d.NotifyCatalogDiff("c", 5, diff)
	d.NotifyCatalogDiff("c", 5, diff)
	due, err := st.ListWebhookOutboxDue(time.Now().Add(time.Hour), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 1 {
		t.Fatalf("want 1 outbox row, got %d", len(due))
	}
	if due[0].DeliveryKey != store.WebhookOutboxDeliveryKey("connector:c", store.WebhookOutboxKindCatalogDiff, 5) {
		t.Fatalf("delivery_key=%q", due[0].DeliveryKey)
	}
}
