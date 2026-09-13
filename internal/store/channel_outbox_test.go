// internal/store/channel_outbox_test.go
package store_test

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func TestMemoryChannelOutboxRoundTrip(t *testing.T) {
	mem := store.NewMemory()
	now := time.Now().UTC().Truncate(time.Millisecond)
	key := store.ChannelOutboxDeliveryKey("weixin", "weixin:a:p", store.ChannelOutboxKindText, "run_1", 1)
	entry := store.ChannelOutboxEntry{
		DeliveryKey:    key,
		Channel:        "weixin",
		Kind:           store.ChannelOutboxKindText,
		PeerID:         "p",
		ConversationID: "weixin:a:p",
		Account:        "a",
		RunID:          "run_1",
		PayloadJSON:    []byte(`{"text":"hi"}`),
		BlobKeysJSON:   []byte(`[]`),
		TargetURL:      "http://127.0.0.1:9/outbound",
		NextRetryAt:    now,
		CreatedAt:      now,
	}
	created, id, err := mem.PutChannelOutboxIfAbsent(entry)
	if err != nil || !created || id == "" {
		t.Fatalf("Put created=%v id=%q err=%v", created, id, err)
	}
	dup, dupID, err := mem.PutChannelOutboxIfAbsent(entry)
	if err != nil || dup || dupID != id {
		t.Fatalf("dup created=%v id=%q err=%v", dup, dupID, err)
	}
	due, err := mem.ListChannelOutboxDue(now.Add(time.Second), 10)
	if err != nil || len(due) != 1 || due[0].ID != id {
		t.Fatalf("due=%+v err=%v", due, err)
	}
	list, err := mem.ListChannelOutbox("weixin", []store.ChannelOutboxStatus{store.ChannelOutboxPending}, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	other, err := mem.ListChannelOutbox("other", []store.ChannelOutboxStatus{store.ChannelOutboxPending}, 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("other channel leak: %+v", other)
	}
	got, err := mem.GetChannelOutbox(id)
	if err != nil || got.DeliveryKey != key {
		t.Fatalf("get=%+v err=%v", got, err)
	}
	got.Status = store.ChannelOutboxDead
	got.LastError = "boom"
	if err := mem.UpdateChannelOutbox(got); err != nil {
		t.Fatal(err)
	}
	if err := mem.ResetChannelOutboxRetry(id); err != nil {
		t.Fatal(err)
	}
	reset, _ := mem.GetChannelOutbox(id)
	if reset.Status != store.ChannelOutboxPending || reset.Attempt != 0 {
		t.Fatalf("reset=%+v", reset)
	}
}
