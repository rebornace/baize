package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

func TestWebhookDeliveryE2E(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Baize-Protocol") != "v0" {
			t.Errorf("protocol=%q", r.Header.Get("X-Baize-Protocol"))
		}
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer receiver.Close()

	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "agent"
	cfg.Agent.System = "test"
	cfg.Run.MaxSteps = 4
	cfg.Events.Webhook.URL = receiver.URL

	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	runBody := `{"agent_id":"agent","input":"hello webhook"}`
	resp, err := http.Post(runtimeURL+"/v0/runs", "application/json", strings.NewReader(runBody))
	if err != nil {
		t.Fatalf("POST /v0/runs: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST status=%d body=%s", resp.StatusCode, raw)
	}
	var created struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}

	pollRun(t, runtimeURL, created.RunID, store.StatusSucceeded)

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(bodies)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(30 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(bodies) < 2 {
		t.Fatalf("webhook bodies=%d %+v", len(bodies), bodies)
	}
	foundEnd := false
	for _, b := range bodies {
		if b["ended"] == true {
			foundEnd = true
			if b["status"] != "succeeded" {
				t.Fatalf("end body=%+v", b)
			}
		}
	}
	if !foundEnd {
		t.Fatalf("missing run.ended webhook: %+v", bodies)
	}
}

func TestEventsWebhookSettingsAPI(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "agent"
	cfg.Agent.System = "test"

	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	putBody := `{"url":"https://example.com/hook","headers":{"Authorization":"env:TEST_HOOK"}}`
	putResp, err := http.NewRequest(http.MethodPut, runtimeURL+"/v0/settings/events-webhook", strings.NewReader(putBody))
	if err != nil {
		t.Fatal(err)
	}
	putResp.Header.Set("Content-Type", "application/json")
	putHTTP, err := http.DefaultClient.Do(putResp)
	if err != nil {
		t.Fatal(err)
	}
	defer putHTTP.Body.Close()
	if putHTTP.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(putHTTP.Body)
		t.Fatalf("PUT status=%d body=%s", putHTTP.StatusCode, raw)
	}

	getResp, err := http.Get(runtimeURL + "/v0/settings/events-webhook")
	if err != nil {
		t.Fatal(err)
	}
	defer getResp.Body.Close()
	raw, _ := io.ReadAll(getResp.Body)
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("GET status=%d body=%s", getResp.StatusCode, raw)
	}
	if !strings.Contains(string(raw), "env:TEST_HOOK") {
		t.Fatalf("GET should echo reference shape: %s", raw)
	}
}
