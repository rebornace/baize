package store_test

import (
	"io"
	"path/filepath"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestMemoryWebhookOutboxRoundTrip(t *testing.T) {
	mem := store.NewMemory()
	now := time.Now().UTC().Truncate(time.Millisecond)

	entry := store.WebhookOutboxEntry{
		RunID:       "run_1",
		Kind:        store.WebhookOutboxKindEvent,
		EventIndex:  0,
		PayloadJSON: []byte(`{"run_id":"run_1"}`),
		TargetURL:   "https://example.com/hook",
		HeadersJSON: []byte(`{}`),
		NextRetryAt: now,
		CreatedAt:   now,
	}
	created, id, err := mem.PutWebhookOutboxIfAbsent(entry)
	if err != nil || !created || id == "" {
		t.Fatalf("Put created=%v id=%q err=%v", created, id, err)
	}

	dupCreated, dupID, err := mem.PutWebhookOutboxIfAbsent(entry)
	if err != nil || dupCreated || dupID != id {
		t.Fatalf("dup created=%v id=%q want %q err=%v", dupCreated, dupID, id, err)
	}

	due, err := mem.ListWebhookOutboxDue(now.Add(time.Second), 10)
	if err != nil || len(due) != 1 || due[0].ID != id {
		t.Fatalf("due=%+v err=%v", due, err)
	}

	got, err := mem.GetWebhookOutbox(id)
	if err != nil {
		t.Fatal(err)
	}
	got.Status = store.WebhookOutboxDead
	got.Attempt = 3
	got.LastError = "HTTP 503"
	if err := mem.UpdateWebhookOutbox(got); err != nil {
		t.Fatal(err)
	}

	listed, err := mem.ListWebhookOutbox([]store.WebhookOutboxStatus{store.WebhookOutboxDead}, 10)
	if err != nil || len(listed) != 1 || listed[0].Status != store.WebhookOutboxDead {
		t.Fatalf("listed=%+v err=%v", listed, err)
	}

	if err := mem.ResetWebhookOutboxRetry(id); err != nil {
		t.Fatal(err)
	}
	reset, err := mem.GetWebhookOutbox(id)
	if err != nil || reset.Status != store.WebhookOutboxPending || reset.Attempt != 0 || reset.LastError != "" {
		t.Fatalf("reset=%+v err=%v", reset, err)
	}
}

func TestSQLiteWebhookOutboxRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webhook-outbox.db")
	s, err := store.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if c, ok := s.(io.Closer); ok {
			_ = c.Close()
		}
	})

	now := time.Now().UTC().Truncate(time.Millisecond)
	entry := store.WebhookOutboxEntry{
		RunID:       "run_sql",
		Kind:        store.WebhookOutboxKindEnded,
		EventIndex:  -1,
		PayloadJSON: []byte(`{"ended":true}`),
		TargetURL:   "https://example.com/end",
		HeadersJSON: []byte(`{"X-Test":"1"}`),
		NextRetryAt: now,
		CreatedAt:   now,
	}
	created, id, err := s.PutWebhookOutboxIfAbsent(entry)
	if err != nil || !created || id == "" {
		t.Fatalf("Put created=%v id=%q err=%v", created, id, err)
	}

	due, err := s.ListWebhookOutboxDue(now.Add(time.Second), 5)
	if err != nil || len(due) != 1 {
		t.Fatalf("due=%+v err=%v", due, err)
	}

	got, err := s.GetWebhookOutbox(id)
	if err != nil {
		t.Fatal(err)
	}
	got.Status = store.WebhookOutboxDead
	got.Attempt = 5
	got.LastError = "HTTP 503"
	if err := s.UpdateWebhookOutbox(got); err != nil {
		t.Fatal(err)
	}

	if err := s.ResetWebhookOutboxRetry(id); err != nil {
		t.Fatal(err)
	}
	reset, err := s.GetWebhookOutbox(id)
	if err != nil || reset.Status != store.WebhookOutboxPending || reset.Attempt != 0 {
		t.Fatalf("reset=%+v err=%v", reset, err)
	}
}
