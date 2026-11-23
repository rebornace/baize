package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/channel"
	"github.com/rebornace/baize/internal/channel/webhook"
	"github.com/rebornace/baize/internal/store"
)

func openWebhookChannel(t *testing.T, name string, mem *store.Memory) *webhook.Channel {
	t.Helper()
	raw, err := channel.Open("webhook", channel.Config{
		"name":         name,
		"secret":       "sec",
		"outbound_url": "http://example.com/outbound",
		"assignee":     "alice",
	})
	if err != nil {
		t.Fatalf("open webhook channel %q: %v", name, err)
	}
	ch, ok := raw.(*webhook.Channel)
	if !ok {
		t.Fatalf("want *webhook.Channel, got %T", raw)
	}
	ch.SetStore(mem)
	return ch
}

func TestChannelOutboundDeliveriesListAndRetry(t *testing.T) {
	mem := store.NewMemory()
	ch := openWebhookChannel(t, "weixin", mem)
	other := openWebhookChannel(t, "other", mem)

	srv := api.NewServer(mem, nil, nil)
	srv.AdminToken = "adm"
	srv.RegisterChannel(&api.ChannelHandle{Name: "weixin", Channel: ch})
	srv.RegisterChannel(&api.ChannelHandle{Name: "other", Channel: other})

	now := time.Now().UTC()
	created, id, err := mem.PutChannelOutboxIfAbsent(store.ChannelOutboxEntry{
		DeliveryKey:    store.ChannelOutboxDeliveryKey("weixin", "weixin:a:p", store.ChannelOutboxKindText, "run_dead", "1"),
		Channel:        "weixin",
		Kind:           store.ChannelOutboxKindText,
		PeerID:         "p",
		ConversationID: "weixin:a:p",
		Account:        "a",
		RunID:          "run_dead",
		PayloadJSON:    []byte(`{"text":"hi","secret":"should-not-leak"}`),
		BlobKeysJSON:   []byte(`[]`),
		TargetURL:      "http://example.com/outbound",
		Status:         store.ChannelOutboxDead,
		Attempt:        5,
		LastError:      "HTTP 503",
		NextRetryAt:    now,
		CreatedAt:      now,
	})
	if err != nil || !created || id == "" {
		t.Fatalf("Put created=%v id=%q err=%v", created, id, err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/weixin/outbound-deliveries", nil)
	getReq.Header.Set("Authorization", "Bearer adm")
	getRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET weixin status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var listed struct {
		Deliveries []struct {
			ID             string `json:"id"`
			Status         string `json:"status"`
			Kind           string `json:"kind"`
			PeerID         string `json:"peer_id"`
			ConversationID string `json:"conversation_id"`
			RunID          string `json:"run_id"`
			Attempt        int    `json:"attempt"`
			MaxAttempts    int    `json:"max_attempts"`
			LastError      string `json:"last_error"`
			UpdatedAt      string `json:"updated_at"`
			PayloadJSON    string `json:"payload_json"`
			TargetURL      string `json:"target_url"`
		} `json:"deliveries"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Deliveries) != 1 || listed.Deliveries[0].ID != id || listed.Deliveries[0].Status != "dead" {
		t.Fatalf("deliveries=%+v", listed.Deliveries)
	}
	if listed.Deliveries[0].PayloadJSON != "" || listed.Deliveries[0].TargetURL != "" {
		t.Fatalf("response must not include payload/secret fields: %+v", listed.Deliveries[0])
	}
	if listed.Deliveries[0].Kind != "text" || listed.Deliveries[0].PeerID != "p" ||
		listed.Deliveries[0].ConversationID != "weixin:a:p" || listed.Deliveries[0].RunID != "run_dead" {
		t.Fatalf("metadata=%+v", listed.Deliveries[0])
	}

	unknownReq := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/unknown/outbound-deliveries", nil)
	unknownReq.Header.Set("Authorization", "Bearer adm")
	unknownRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unknownRR, unknownReq)
	if unknownRR.Code != http.StatusNotFound {
		t.Fatalf("GET unknown channel status=%d want 404 body=%s", unknownRR.Code, unknownRR.Body.String())
	}

	otherListReq := httptest.NewRequest(http.MethodGet, "/v0/settings/channels/other/outbound-deliveries", nil)
	otherListReq.Header.Set("Authorization", "Bearer adm")
	otherListRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(otherListRR, otherListReq)
	if otherListRR.Code != http.StatusOK {
		t.Fatalf("GET other status=%d body=%s", otherListRR.Code, otherListRR.Body.String())
	}
	var otherListed struct {
		Deliveries []struct {
			ID string `json:"id"`
		} `json:"deliveries"`
	}
	if err := json.Unmarshal(otherListRR.Body.Bytes(), &otherListed); err != nil {
		t.Fatal(err)
	}
	if len(otherListed.Deliveries) != 0 {
		t.Fatalf("other channel must not see weixin deliveries: %+v", otherListed.Deliveries)
	}

	wrongChReq := httptest.NewRequest(http.MethodPost, "/v0/settings/channels/other/outbound-deliveries/"+id+"/retry", nil)
	wrongChReq.Header.Set("Authorization", "Bearer adm")
	wrongChRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wrongChRR, wrongChReq)
	if wrongChRR.Code != http.StatusNotFound {
		t.Fatalf("POST wrong channel id status=%d want 404 body=%s", wrongChRR.Code, wrongChRR.Body.String())
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/v0/settings/channels/weixin/outbound-deliveries/"+id+"/retry", nil)
	retryReq.Header.Set("Authorization", "Bearer adm")
	retryRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(retryRR, retryReq)
	if retryRR.Code != http.StatusOK {
		t.Fatalf("POST retry status=%d body=%s", retryRR.Code, retryRR.Body.String())
	}
	got, err := mem.GetChannelOutbox(id)
	if err != nil || got.Status != store.ChannelOutboxPending || got.Attempt != 0 {
		t.Fatalf("after retry got=%+v err=%v", got, err)
	}
}
