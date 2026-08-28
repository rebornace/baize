package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/inbox"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

// TestInboxResumeApproveE2E: Inbox create → waiting_human → Inbox resume approve → succeeded,
// then idempotent resume replay returns 200 without changing status.
func TestInboxResumeApproveE2E(t *testing.T) {
	cfg := inboxResumeConfig(t)
	runtimeURL, ticketURL, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	before := ticketCount(t, ticketURL)

	createBody := []byte(`{"input":"创建一个紧急工单：Inbox HITL resume"}`)
	createResp, err := signInboxPOST(runtimeURL, "alerts", inboxTestSecret, createBody)
	if err != nil {
		t.Fatalf("POST inbox create: %v", err)
	}
	if createResp.StatusCode != http.StatusAccepted {
		raw, _ := io.ReadAll(createResp.Body)
		createResp.Body.Close()
		t.Fatalf("POST inbox create status=%d body=%s", createResp.StatusCode, raw)
	}
	accepted := decodeInboxAccepted(t, createResp)
	if accepted.RunID == "" {
		t.Fatalf("missing run_id: %+v", accepted)
	}

	pollRun(t, runtimeURL, accepted.RunID, store.StatusWaitingHuman)

	resumeBody := []byte(`{"action":"resume","run_id":"` + accepted.RunID + `","decision":"approve","comment":"inbox ok","idempotency_key":"inbox-resume-e2e-1"}`)
	resumeResp, err := signInboxPOST(runtimeURL, "alerts", inboxTestSecret, resumeBody)
	if err != nil {
		t.Fatalf("POST inbox resume: %v", err)
	}
	resumeOut := decodeInboxResume(t, resumeResp)
	if resumeResp.StatusCode != http.StatusOK {
		t.Fatalf("POST inbox resume status=%d body=%+v", resumeResp.StatusCode, resumeOut)
	}
	if resumeOut.Action != "resume" || resumeOut.RunID != accepted.RunID {
		t.Fatalf("resume resp=%+v want action=resume run_id=%s", resumeOut, accepted.RunID)
	}
	if resumeOut.DeliveryID == "" {
		t.Fatalf("missing delivery_id: %+v", resumeOut)
	}

	pollRun(t, runtimeURL, accepted.RunID, store.StatusSucceeded)

	after := ticketCount(t, ticketURL)
	if after < before+1 {
		t.Fatalf("tickets after approve=%d before=%d want >= before+1", after, before)
	}

	assertInboxResumedEvent(t, runtimeURL, accepted.RunID)

	replayResp, err := signInboxPOST(runtimeURL, "alerts", inboxTestSecret, resumeBody)
	if err != nil {
		t.Fatalf("POST inbox resume replay: %v", err)
	}
	replayOut := decodeInboxResume(t, replayResp)
	if replayResp.StatusCode != http.StatusOK {
		t.Fatalf("replay status=%d body=%+v", replayResp.StatusCode, replayOut)
	}
	if replayOut.RunID != accepted.RunID {
		t.Fatalf("replay run_id=%q want %q", replayOut.RunID, accepted.RunID)
	}
	if replayOut.DeliveryID != resumeOut.DeliveryID {
		t.Fatalf("replay delivery_id=%q want %q", replayOut.DeliveryID, resumeOut.DeliveryID)
	}
	if replayOut.Status != string(store.StatusSucceeded) {
		t.Fatalf("replay status=%q want %s", replayOut.Status, store.StatusSucceeded)
	}

	final := getRunStatus(t, runtimeURL, accepted.RunID)
	if final != store.StatusSucceeded {
		t.Fatalf("run status after replay=%q want %s", final, store.StatusSucceeded)
	}
}

type inboxResumeAccepted struct {
	DeliveryID string `json:"delivery_id"`
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Action     string `json:"action"`
}

func inboxResumeConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "ticket-agent"
	cfg.Agent.System = "你是企业工单助手，只能通过工具访问工单系统。"
	cfg.Connector.ID = "ticket-api"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = filepath.Join("..", "..", "examples", "mock-ticket", "openapi.yaml")
	cfg.Connector.RequireApproval = []string{"create_ticket"}
	cfg.Run.MaxSteps = 8
	cfg.UI.Enabled = true
	cfg.Inbox.Channels = []inbox.Channel{{
		ID:      "alerts",
		AgentID: "ticket-agent",
		Secret:  inboxTestSecret,
		Enabled: true,
	}}
	return cfg
}

func decodeInboxResume(t *testing.T, resp *http.Response) inboxResumeAccepted {
	t.Helper()
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var out inboxResumeAccepted
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode inbox resume: %v body=%s", err, raw)
	}
	return out
}

func getRunStatus(t *testing.T, runtimeURL, runID string) store.Status {
	t.Helper()
	resp, err := http.Get(runtimeURL + "/v0/runs/" + runID)
	if err != nil {
		t.Fatalf("GET run: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET run status=%d body=%s", resp.StatusCode, raw)
	}
	var runRec store.Run
	if err := json.Unmarshal(raw, &runRec); err != nil {
		t.Fatalf("decode run: %v body=%s", err, raw)
	}
	return runRec.Status
}

func assertInboxResumedEvent(t *testing.T, runtimeURL, runID string) {
	t.Helper()
	evResp, err := http.Get(runtimeURL + "/v0/runs/" + runID + "/events")
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
	for _, ev := range events {
		if ev.Type == run.EventInboxResumed {
			return
		}
	}
	t.Fatalf("events missing %q: %+v", run.EventInboxResumed, events)
}
