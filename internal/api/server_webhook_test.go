package api_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/webhook"
)

func TestEventsWebhookSettingsRoundTrip(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	d := webhook.NewDispatcher(st, webhook.Config{})
	d.Attach(hub)
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.Hub = hub
	srv.Webhook = d

	putBody := map[string]any{
		"url": "https://example.com/hook",
		"headers": map[string]string{
			"Authorization": "env:HOOK_TOKEN",
		},
	}
	putReq := httptest.NewRequest(http.MethodPut, "/v0/settings/events-webhook", jsonBody(t, putBody))
	putRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putRR, putReq)
	if putRR.Code != http.StatusOK {
		t.Fatalf("PUT status=%d body=%s", putRR.Code, putRR.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/settings/events-webhook", nil)
	getRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var got webhook.Config
	if err := json.Unmarshal(getRR.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.URL != putBody["url"] {
		t.Fatalf("url=%q", got.URL)
	}
	if got.Headers["Authorization"] != "env:HOOK_TOKEN" {
		t.Fatalf("headers=%+v", got.Headers)
	}
}

func TestPostRunWebhookNotEchoedOnGetRun(t *testing.T) {
	mem := store.NewMemory()
	srv := api.NewServer(mem, tool.NewRegistry(), &fakeRunner{store: mem})
	srv.Store.UpsertAgent(store.Agent{ID: "a1", System: "sys"})

	postReq := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, map[string]any{
		"agent_id":        "a1",
		"input":           "hi",
		"webhook_url":     "https://secret.example/hook",
		"webhook_headers": map[string]string{"X-Test": "1"},
	}))
	postRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRR, postReq)
	if postRR.Code != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", postRR.Code, postRR.Body.String())
	}
	var created struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(postRR.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/runs/"+created.RunID, nil)
	getRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	if strings.Contains(getRR.Body.String(), "webhook") {
		t.Fatalf("GET run leaked webhook: %s", getRR.Body.String())
	}

	run, err := mem.GetRun(created.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if run.WebhookConfig == nil || run.WebhookConfig.URL != "https://secret.example/hook" {
		t.Fatalf("stored webhook=%+v", run.WebhookConfig)
	}
}

func TestGetEventsWebhookDeliveriesAndRetry(t *testing.T) {
	mem := store.NewMemory()
	hub := eventbus.NewHub()
	st := eventbus.Notify(mem, hub)
	d := webhook.NewDispatcher(st, webhook.Config{})
	d.Attach(hub)
	srv := api.NewServer(st, tool.NewRegistry(), &fakeRunner{store: st})
	srv.Hub = hub
	srv.Webhook = d

	now := time.Now().UTC()
	created, id, err := st.PutWebhookOutboxIfAbsent(store.WebhookOutboxEntry{
		RunID:       "run_dead",
		Kind:        store.WebhookOutboxKindEvent,
		EventIndex:  1,
		PayloadJSON: []byte(`{"run_id":"run_dead"}`),
		TargetURL:   "https://example.com/hook",
		HeadersJSON: []byte(`{}`),
		Status:      store.WebhookOutboxDead,
		Attempt:     5,
		LastError:   "HTTP 503",
		NextRetryAt: now,
		CreatedAt:   now,
	})
	if err != nil || !created {
		t.Fatalf("Put created=%v err=%v", created, err)
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v0/settings/events-webhook/deliveries", nil)
	getRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRR, getReq)
	if getRR.Code != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getRR.Code, getRR.Body.String())
	}
	var listed struct {
		Deliveries []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"deliveries"`
	}
	if err := json.Unmarshal(getRR.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Deliveries) != 1 || listed.Deliveries[0].ID != id || listed.Deliveries[0].Status != "dead" {
		t.Fatalf("deliveries=%+v", listed.Deliveries)
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/v0/settings/events-webhook/deliveries/"+id+"/retry", nil)
	retryRR := httptest.NewRecorder()
	srv.Handler().ServeHTTP(retryRR, retryReq)
	if retryRR.Code != http.StatusOK {
		t.Fatalf("POST retry status=%d body=%s", retryRR.Code, retryRR.Body.String())
	}

	got, err := st.GetWebhookOutbox(id)
	if err != nil || got.Status != store.WebhookOutboxPending || got.Attempt != 0 {
		t.Fatalf("after retry got=%+v err=%v", got, err)
	}
}
