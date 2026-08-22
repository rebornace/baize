package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/store"
)

func TestCreateTicketE2E(t *testing.T) {
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "ticket-agent"
	cfg.Agent.System = "你是企业工单助手，只能通过工具访问工单系统。"
	// Out-of-box default: hang ticket-triage so visible tools are Skill∩enabled
	// (still includes create_ticket). Empty skills would expose all enabled tools.
	cfg.Skills.BuiltinDir = filepath.Join("..", "..", "examples", "skills")
	cfg.Skills.UserDir = t.TempDir()
	cfg.Agent.Skills = []string{"ticket-triage"}
	cfg.Connector.ID = "ticket-api"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = filepath.Join("..", "..", "examples", "mock-ticket", "openapi.yaml")
	cfg.Connector.RequireApproval = []string{"create_ticket"}
	cfg.Run.MaxSteps = 8

	runtimeURL, ticketURL, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	runBody := `{"agent_id":"ticket-agent","input":"创建一个紧急工单：VPN 挂了"}`
	resp, err := http.Post(runtimeURL+"/v0/runs", "application/json", strings.NewReader(runBody))
	if err != nil {
		t.Fatalf("POST /v0/runs: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v0/runs status=%d body=%s", resp.StatusCode, raw)
	}
	var created struct {
		RunID  string `json:"run_id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode run: %v body=%s", err, raw)
	}
	if created.RunID == "" {
		t.Fatalf("missing run_id: %s", raw)
	}

	pollRun(t, runtimeURL, created.RunID, store.StatusWaitingHuman)

	resumeBody := `{"decision":"approve","comment":"ok"}`
	resumeResp, err := http.Post(runtimeURL+"/v0/runs/"+created.RunID+"/resume", "application/json", strings.NewReader(resumeBody))
	if err != nil {
		t.Fatalf("POST resume: %v", err)
	}
	defer resumeResp.Body.Close()
	resumeRaw, _ := io.ReadAll(resumeResp.Body)
	if resumeResp.StatusCode != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", resumeResp.StatusCode, resumeRaw)
	}

	pollRun(t, runtimeURL, created.RunID, store.StatusSucceeded)

	evResp, err := http.Get(runtimeURL + "/v0/runs/" + created.RunID + "/events")
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
	hasToolResult := false
	for _, ev := range events {
		if ev.Type == "tool.result" {
			hasToolResult = true
			break
		}
	}
	if !hasToolResult {
		t.Fatalf("events missing tool.result: %+v", events)
	}

	ticketResp, err := http.Get(ticketURL + "/tickets")
	if err != nil {
		t.Fatalf("GET tickets: %v", err)
	}
	defer ticketResp.Body.Close()
	ticketRaw, _ := io.ReadAll(ticketResp.Body)
	if ticketResp.StatusCode != http.StatusOK {
		t.Fatalf("GET tickets status=%d body=%s", ticketResp.StatusCode, ticketRaw)
	}
	var tickets []map[string]any
	if err := json.Unmarshal(ticketRaw, &tickets); err != nil {
		t.Fatalf("decode tickets: %v body=%s", err, ticketRaw)
	}
	if len(tickets) < 1 {
		t.Fatalf("tickets len=%d want >=1; body=%s", len(tickets), ticketRaw)
	}
}

func pollRun(t *testing.T, runtimeURL, runID string, want store.Status) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	var last string
	for time.Now().Before(deadline) {
		resp, err := http.Get(runtimeURL + "/v0/runs/" + runID)
		if err != nil {
			t.Fatalf("GET run: %v", err)
		}
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		var run store.Run
		if err := json.Unmarshal(raw, &run); err != nil {
			t.Fatalf("decode run: %v body=%s", err, raw)
		}
		last = string(run.Status)
		if run.Status == want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("run status=%q want %s", last, want)
}
