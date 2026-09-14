package webhook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/store"
)

func newOutboxTestChannel(t *testing.T, outboundURL string) (*Channel, *store.Memory) {
	t.Helper()
	mem := store.NewMemory()
	ch := &Channel{
		cfg: instanceConfig{
			Name:           "weixin",
			Source:         "weixin",
			Account:        "acc",
			OutboundURL:    outboundURL,
			OutboundSecret: "sec",
			Secret:         "sec",
		},
		persist:    mem,
		outboxWake: make(chan struct{}, 1),
	}
	ch.sender = newSender(&ch.cfg)
	return ch, mem
}

func startOutboxWorker(t *testing.T, ch *Channel) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go ch.StartOutboxWorker(ctx)
}

func waitChannelOutbox(t *testing.T, mem *store.Memory, name string, status store.ChannelOutboxStatus, timeout time.Duration) []store.ChannelOutboxEntry {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		list, err := mem.ListChannelOutbox(name, []store.ChannelOutboxStatus{status}, 5)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		if len(list) > 0 {
			return list
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for channel=%s status=%s", name, status)
	return nil
}

func TestOutboxWorkerRetriesThenDelivers(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := hits.Add(1)
		if n < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ch, mem := newOutboxTestChannel(t, srv.URL)
	startOutboxWorker(t, ch)

	if err := ch.SendText(context.Background(), "peer1", "hello", map[string]string{
		channel.ExtraKind: channel.OutboundKindAssistant,
		"run_id":          "run_x",
	}); err != nil {
		t.Fatal(err)
	}
	list := waitChannelOutbox(t, mem, "weixin", store.ChannelOutboxDelivered, 8*time.Second)
	if hits.Load() < 3 {
		t.Fatalf("hits=%d want >= 3", hits.Load())
	}
	if list[0].RunID != "run_x" {
		t.Fatalf("run_id=%q", list[0].RunID)
	}
}

func TestOutboxWorkerFourxxGoesDead(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	t.Cleanup(srv.Close)

	ch, mem := newOutboxTestChannel(t, srv.URL)
	startOutboxWorker(t, ch)

	if err := ch.SendText(context.Background(), "peer1", "nope", map[string]string{
		channel.ExtraKind: channel.OutboundKindAssistant,
		"run_id":          "run_4xx",
	}); err != nil {
		t.Fatal(err)
	}
	list := waitChannelOutbox(t, mem, "weixin", store.ChannelOutboxDead, 5*time.Second)
	if hits.Load() != 1 {
		t.Fatalf("4xx must not retry, hits=%d", hits.Load())
	}
	if list[0].Attempt < 1 {
		t.Fatalf("attempt=%d", list[0].Attempt)
	}
}

func TestOutboxSendMediaUsesBlob(t *testing.T) {
	wantBytes := []byte("png-bytes-xyz")
	var gotBody []byte
	done := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		select {
		case done <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ch, mem := newOutboxTestChannel(t, srv.URL)
	ch.blobs = blobs
	startOutboxWorker(t, ch)

	if err := ch.SendMedia(context.Background(), "peer1", "pic.png", "image/png", wantBytes, map[string]string{
		channel.ExtraKind: channel.OutboundKindAssistant,
		"run_id":          "run_media",
	}); err != nil {
		t.Fatal(err)
	}
	_ = waitChannelOutbox(t, mem, "weixin", store.ChannelOutboxDelivered, 5*time.Second)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("POST not received")
	}
	var msg OutboundMessage
	if err := json.Unmarshal(gotBody, &msg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(msg.Media) != 1 {
		t.Fatalf("media len=%d body=%s", len(msg.Media), gotBody)
	}
	decoded, err := base64.StdEncoding.DecodeString(msg.Media[0].ContentBase64)
	if err != nil {
		t.Fatal(err)
	}
	if string(decoded) != string(wantBytes) {
		t.Fatalf("decoded=%q want %q", decoded, wantBytes)
	}
	if msg.Media[0].Name != "pic.png" || msg.Media[0].MIME != "image/png" {
		t.Fatalf("media meta: %+v", msg.Media[0])
	}
}

// TestEnqueueAfterRestartDoesNotDropWhenDeliveredHistoryExists locks the
// regression: process-local seq restart must not collide with a delivered
// delivery_key and silently drop a new SendText (same conv, empty/_ run_id).
func TestEnqueueAfterRestartDoesNotDropWhenDeliveredHistoryExists(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	ch, mem := newOutboxTestChannel(t, srv.URL)
	ch.cfg.Source = "weixin"
	startOutboxWorker(t, ch)

	convID := channel.ConvID(ch.cfg.Source, ch.cfg.Account, "peer1")
	// Old seq-based key that a restarted process would regenerate as :1.
	legacyKey := store.ChannelOutboxDeliveryKey(ch.cfg.Name, convID, store.ChannelOutboxKindText, "", "1")
	now := time.Now().UTC()
	created, presetID, err := mem.PutChannelOutboxIfAbsent(store.ChannelOutboxEntry{
		DeliveryKey:    legacyKey,
		Channel:        ch.cfg.Name,
		Kind:           store.ChannelOutboxKindText,
		PeerID:         "peer1",
		ConversationID: convID,
		Account:        ch.cfg.Account,
		RunID:          "",
		PayloadJSON:    []byte(`{"text":"old"}`),
		BlobKeysJSON:   []byte(`[]`),
		TargetURL:      srv.URL,
		Status:         store.ChannelOutboxDelivered,
		NextRetryAt:    now,
		CreatedAt:      now,
	})
	if err != nil || !created || presetID == "" {
		t.Fatalf("preset delivered created=%v id=%q err=%v", created, presetID, err)
	}

	// Simulate process restart: fresh Channel (outboxSeq would have been 0)
	// sharing the same durable store with delivered history.
	ch2 := &Channel{
		cfg:        ch.cfg,
		persist:    mem,
		outboxWake: make(chan struct{}, 1),
	}
	ch2.sender = newSender(&ch2.cfg)
	startOutboxWorker(t, ch2)

	if err := ch2.SendText(context.Background(), "peer1", "after-restart", map[string]string{
		channel.ExtraKind: channel.OutboundKindAssistant,
		// empty run_id → delivery_key uses "_"
	}); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	var newRow *store.ChannelOutboxEntry
	for time.Now().Before(deadline) {
		list, err := mem.ListChannelOutbox(ch.cfg.Name, []store.ChannelOutboxStatus{
			store.ChannelOutboxPending, store.ChannelOutboxDelivered,
		}, 20)
		if err != nil {
			t.Fatalf("list: %v", err)
		}
		for i := range list {
			if list[i].ID != presetID {
				e := list[i]
				newRow = &e
				break
			}
		}
		if newRow != nil && newRow.Status == store.ChannelOutboxDelivered {
			break
		}
		if newRow != nil && hits.Load() >= 1 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if newRow == nil {
		t.Fatal("SendText after restart must enqueue a new outbox row; got only preset delivered")
	}
	if newRow.DeliveryKey == legacyKey {
		t.Fatalf("new delivery_key must differ from legacy seq key %q", legacyKey)
	}
	if hits.Load() < 1 {
		t.Fatalf("expected outbound delivery, hits=%d", hits.Load())
	}
}

func TestPutMediaBlobRejectsPathTraversalInFilename(t *testing.T) {
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	ch, _ := newOutboxTestChannel(t, "http://127.0.0.1:9/outbound")
	ch.blobs = blobs

	key, err := ch.putMediaBlob(context.Background(), "../secret/../../evil.png", "image/png", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(key, "..") {
		t.Fatalf("blob key must not contain ..: %q", key)
	}
	if !strings.HasPrefix(key, "channel-outbox/weixin/") {
		t.Fatalf("key prefix: %q", key)
	}
	if !strings.HasSuffix(key, "/evil.png") {
		t.Fatalf("expected Base filename in key, got %q", key)
	}
	parts := strings.Split(key, "/")
	if len(parts) != 4 {
		t.Fatalf("key must be channel-outbox/{ch}/{uuid}/{file}, got %q (%d parts)", key, len(parts))
	}
}
