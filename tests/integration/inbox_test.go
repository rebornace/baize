package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

const inboxTestSecret = "integration-test-inbox-secret-12345"

func inboxTestConfig() config.Config {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "inbox-agent"
	cfg.Agent.System = "test inbox integration agent"
	cfg.Run.MaxSteps = 4
	cfg.Inbox.Channels = []inbox.Channel{{
		ID:      "alerts",
		AgentID: "inbox-agent",
		Secret:  inboxTestSecret,
		Enabled: true,
	}}
	return cfg
}

type inboxAccepted struct {
	DeliveryID     string `json:"delivery_id"`
	RunID          string `json:"run_id"`
	ConversationID string `json:"conversation_id"`
	Status         string `json:"status"`
}

func signInboxPOST(runtimeURL, channelID, secret string, body []byte) (*http.Response, error) {
	ts := strconv.FormatInt(time.Now().Unix(), 10)
	sig := inbox.Sign(secret, ts, body)
	req, err := http.NewRequest(http.MethodPost, runtimeURL+"/v0/inbox/"+channelID, strings.NewReader(string(body)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Baize-Inbox-Timestamp", ts)
	req.Header.Set("X-Baize-Inbox-Signature", sig)
	return http.DefaultClient.Do(req)
}

func decodeInboxAccepted(t *testing.T, resp *http.Response) inboxAccepted {
	t.Helper()
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out inboxAccepted
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode inbox response: %v body=%s", err, raw)
	}
	return out
}

func putInboxChannels(t *testing.T, runtimeURL, agentID, channelID, secret string) {
	t.Helper()
	putBody := `{"channels":[{"id":"` + channelID + `","agent_id":"` + agentID + `","enabled":true,"secret":"` + secret + `"}]}`
	req, err := http.NewRequest(http.MethodPut, runtimeURL+"/v0/settings/inbox-channels", strings.NewReader(putBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT inbox-channels status=%d body=%s", resp.StatusCode, raw)
	}
}

func TestInboxE2ECreatesRun(t *testing.T) {
	cfg := inboxTestConfig()
	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	body := []byte(`{"input":"hello from inbox"}`)
	resp, err := signInboxPOST(runtimeURL, "alerts", inboxTestSecret, body)
	if err != nil {
		t.Fatalf("POST inbox: %v", err)
	}
	if resp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("POST inbox status=%d body=%s", resp.StatusCode, raw)
	}
	accepted := decodeInboxAccepted(t, resp)
	if accepted.RunID == "" || accepted.DeliveryID == "" {
		t.Fatalf("missing run_id or delivery_id: %+v", accepted)
	}
	if !strings.HasPrefix(accepted.DeliveryID, "dlv_") {
		t.Fatalf("delivery_id=%q", accepted.DeliveryID)
	}
	if accepted.Status != "accepted" {
		t.Fatalf("status=%q want accepted", accepted.Status)
	}

	pollRun(t, runtimeURL, accepted.RunID, store.StatusSucceeded)

	evResp, err := http.Get(runtimeURL + "/v0/runs/" + accepted.RunID + "/events")
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer evResp.Body.Close()
	evRaw, _ := io.ReadAll(evResp.Body)
	if evResp.StatusCode != http.StatusOK {
		t.Fatalf("GET events status=%d body=%s", evResp.StatusCode, evRaw)
	}
	var events []store.Event
	if err := json.Unmarshal(evRaw, &events); err != nil {
		t.Fatalf("decode events: %v body=%s", err, evRaw)
	}
	foundInbox := false
	foundStarted := false
	for _, ev := range events {
		if ev.Type == run.EventInboxReceived {
			foundInbox = true
		}
		if ev.Type == run.EventRunStarted {
			foundStarted = true
		}
	}
	if !foundInbox {
		t.Fatalf("events missing %q: %+v", run.EventInboxReceived, events)
	}
	if !foundStarted {
		t.Fatalf("events missing run.started: %+v", events)
	}
}

func TestInboxIdempotencyReplay(t *testing.T) {
	cfg := inboxTestConfig()
	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	body := []byte(`{"input":"idempotent message","idempotency_key":"idem-integration-1"}`)

	resp1, err := signInboxPOST(runtimeURL, "alerts", inboxTestSecret, body)
	if err != nil {
		t.Fatalf("first POST inbox: %v", err)
	}
	if resp1.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp1.Body)
		resp1.Body.Close()
		t.Fatalf("first POST status=%d body=%s", resp1.StatusCode, raw)
	}
	first := decodeInboxAccepted(t, resp1)
	if first.RunID == "" || first.DeliveryID == "" {
		t.Fatalf("first response missing ids: %+v", first)
	}

	resp2, err := signInboxPOST(runtimeURL, "alerts", inboxTestSecret, body)
	if err != nil {
		t.Fatalf("second POST inbox: %v", err)
	}
	if resp2.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		t.Fatalf("replay POST status=%d want 200 body=%s", resp2.StatusCode, raw)
	}
	second := decodeInboxAccepted(t, resp2)
	if second.RunID != first.RunID {
		t.Fatalf("run_id=%q want %q", second.RunID, first.RunID)
	}
	if second.DeliveryID != first.DeliveryID {
		t.Fatalf("delivery_id=%q want %q", second.DeliveryID, first.DeliveryID)
	}
}

func TestInboxExternalIDThreadsConversation(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "inbox-agent"
	cfg.Agent.System = "test inbox integration agent"
	cfg.Run.MaxSteps = 4

	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()
	putInboxChannels(t, runtimeURL, "inbox-agent", "alerts", inboxTestSecret)

	body1 := []byte(`{"input":"thread message one","external_id":"ext-thread-42"}`)
	resp1, err := signInboxPOST(runtimeURL, "alerts", inboxTestSecret, body1)
	if err != nil {
		t.Fatalf("first POST inbox: %v", err)
	}
	if resp1.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp1.Body)
		resp1.Body.Close()
		t.Fatalf("first POST status=%d body=%s", resp1.StatusCode, raw)
	}
	first := decodeInboxAccepted(t, resp1)
	if first.ConversationID == "" {
		t.Fatalf("first response missing conversation_id: %+v", first)
	}

	body2 := []byte(`{"input":"thread message two","external_id":"ext-thread-42"}`)
	resp2, err := signInboxPOST(runtimeURL, "alerts", inboxTestSecret, body2)
	if err != nil {
		t.Fatalf("second POST inbox: %v", err)
	}
	if resp2.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(resp2.Body)
		resp2.Body.Close()
		t.Fatalf("second POST status=%d body=%s", resp2.StatusCode, raw)
	}
	second := decodeInboxAccepted(t, resp2)
	if second.ConversationID != first.ConversationID {
		t.Fatalf("conversation_id=%q want %q", second.ConversationID, first.ConversationID)
	}
	if second.RunID == first.RunID {
		t.Fatalf("expected distinct runs for threaded messages, got same run_id=%q", second.RunID)
	}
}
