package api_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/channel/webhook"
	"github.com/rebornace/baize/internal/store"
)

// BenchmarkPerfOutboxList times GET outbound-deliveries list with K=200 seeded
// outbox rows. List only — no Send / no real outbound HTTP.
func BenchmarkPerfOutboxList(b *testing.B) {
	mem := store.NewMemory()
	raw, err := channel.Open("webhook", channel.Config{
		"name":         "weixin",
		"secret":       "sec",
		"outbound_url": "http://example.com/outbound",
		"assignee":     "alice",
	})
	if err != nil {
		b.Fatal(err)
	}
	ch, ok := raw.(*webhook.Channel)
	if !ok {
		b.Fatalf("want *webhook.Channel, got %T", raw)
	}
	ch.SetStore(mem)

	srv := api.NewServer(mem, nil, nil)
	srv.AdminToken = "adm"
	srv.RegisterChannel(&api.ChannelHandle{Name: "weixin", Channel: ch})

	now := time.Now().UTC()
	const k = 200
	for i := 0; i < k; i++ {
		uid := fmt.Sprintf("%d", i+1)
		created, id, err := mem.PutChannelOutboxIfAbsent(store.ChannelOutboxEntry{
			DeliveryKey:    store.ChannelOutboxDeliveryKey("weixin", "weixin:a:p", store.ChannelOutboxKindText, "run_perf", uid),
			Channel:        "weixin",
			Kind:           store.ChannelOutboxKindText,
			PeerID:         "p",
			ConversationID: "weixin:a:p",
			Account:        "a",
			RunID:          "run_perf",
			PayloadJSON:    []byte(`{"text":"hi"}`),
			BlobKeysJSON:   []byte(`[]`),
			TargetURL:      "http://example.com/outbound",
			Status:         store.ChannelOutboxDead,
			Attempt:        1,
			LastError:      "bench",
			NextRetryAt:    now,
			CreatedAt:      now,
		})
		if err != nil || !created || id == "" {
			b.Fatalf("seed %d: created=%v id=%q err=%v", i, created, id, err)
		}
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/weixin/outbound-deliveries", nil)
		req.Header.Set("Authorization", "Bearer adm")
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			b.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
		}
	}
}
