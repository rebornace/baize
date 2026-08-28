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

func TestWebhookRetry503Then200(t *testing.T) {
	var mu sync.Mutex
	var calls int
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		n := calls
		mu.Unlock()
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
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

	runBody := `{"agent_id":"agent","input":"retry webhook"}`
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

	deadline := time.Now().Add(12 * time.Second)
	for {
		mu.Lock()
		n := calls
		mu.Unlock()
		if n >= 3 || time.Now().After(deadline) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if calls < 3 {
		t.Fatalf("webhook calls=%d want >=3", calls)
	}
}

func TestWebhookPermanentFailureDeadLetter(t *testing.T) {
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
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

	runBody := `{"agent_id":"agent","input":"dead webhook"}`
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

	deadline := time.Now().Add(5 * time.Second)
	var listed struct {
		Deliveries []struct {
			Status string `json:"status"`
			RunID  string `json:"run_id"`
		} `json:"deliveries"`
	}
	for {
		getResp, err := http.Get(runtimeURL + "/v0/settings/events-webhook/deliveries?status=dead")
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(getResp.Body)
		getResp.Body.Close()
		if getResp.StatusCode != http.StatusOK {
			t.Fatalf("GET deliveries status=%d body=%s", getResp.StatusCode, body)
		}
		listed.Deliveries = nil
		if err := json.Unmarshal(body, &listed); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, d := range listed.Deliveries {
			if d.RunID == created.RunID && d.Status == "dead" {
				found = true
				break
			}
		}
		if found || time.Now().After(deadline) {
			if !found {
				t.Fatalf("missing dead delivery for run %s: %+v", created.RunID, listed.Deliveries)
			}
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
}
