package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/store"
)

const workflowPipelineSkill = `---
name: ticket-triage
description: integration workflow fixture
tools:
  - list_tickets
  - create_ticket
---

# Workflow fixture

The workflow below is intentionally local to this test; the shipped
ticket-triage example is prompt-only.
`

const workflowPipelineYAML = `name: ticket-triage
steps:
  - id: list
    tool: list_tickets
  - id: create
    tool: create_ticket
    approve: true
    args:
      title: "{{input.text}}"
`

func TestWorkflowPipelineEventsE2E(t *testing.T) {
	cfg := workflowPipelineConfig(t)
	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	runID := startRun(t, runtimeURL, "激活流水线：VPN故障")

	streamCh := make(chan string, 1)
	go func() {
		streamCh <- readRunStreamUntil(t, runtimeURL, runID, "workflow.step_completed", 8*time.Second)
	}()

	pollRun(t, runtimeURL, runID, store.StatusWaitingHuman)

	streamBody := <-streamCh
	for _, typ := range []string{"workflow.started", "workflow.step_started", "workflow.step_completed"} {
		if !strings.Contains(streamBody, typ) {
			t.Fatalf("SSE missing %q; body=%s", typ, streamBody)
		}
	}

	resumeRun(t, runtimeURL, runID, "approve", "ok")
	pollRun(t, runtimeURL, runID, store.StatusSucceeded)

	events := listRunEvents(t, runtimeURL, runID)
	seq := eventTypes(events)
	wantPrefix := []string{
		"model.routed",
		"run.started",
		"run.binding",
		"tool.result",
		"workflow.started",
		"workflow.step_started",
		"llm.tool_call",
		"tool.result",
		"workflow.step_completed",
		"hitl.waiting",
	}
	if len(seq) < len(wantPrefix) {
		t.Fatalf("events too short: %v", seq)
	}
	for i, w := range wantPrefix {
		if seq[i] != w {
			t.Fatalf("event[%d]=%s want %s; full=%v", i, seq[i], w, seq)
		}
	}
}

func workflowPipelineConfig(t *testing.T) config.Config {
	t.Helper()
	cfg := config.Config{}
	cfg.LLM.Provider = "mock"
	cfg.Agent.ID = "ticket-agent"
	cfg.Agent.System = "你是企业工单助手。"
	skillRoot := t.TempDir()
	skillDir := filepath.Join(skillRoot, "ticket-triage")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(workflowPipelineSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "workflow.yaml"), []byte(workflowPipelineYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg.Skills.BuiltinDir = skillRoot
	cfg.Skills.UserDir = t.TempDir()
	cfg.Agent.Skills = []string{"ticket-triage"}
	cfg.Connector.ID = "ticket-api"
	cfg.Connector.Type = "openapi"
	cfg.Connector.Spec = filepath.Join("..", "..", "examples", "mock-ticket", "openapi.yaml")
	cfg.Connector.RequireApproval = []string{"create_ticket"}
	cfg.Run.MaxSteps = 8
	return cfg
}

func listRunEvents(t *testing.T, runtimeURL, runID string) []store.Event {
	t.Helper()
	resp, err := http.Get(runtimeURL + "/v0/runs/" + runID + "/events")
	if err != nil {
		t.Fatalf("GET events: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET events status=%d body=%s", resp.StatusCode, raw)
	}
	var events []store.Event
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatalf("decode events: %v body=%s", err, raw)
	}
	return events
}

func eventTypes(events []store.Event) []string {
	out := make([]string, 0, len(events))
	for _, ev := range events {
		out = append(out, ev.Type)
	}
	return out
}

func readRunStreamUntil(t *testing.T, runtimeURL, runID, needle string, timeout time.Duration) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, runtimeURL+"/v0/runs/"+runID+"/stream?after=-1", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET stream: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("stream status=%d body=%s", resp.StatusCode, raw)
	}
	var b strings.Builder
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		line := sc.Text()
		b.WriteString(line)
		b.WriteByte('\n')
		if strings.Contains(line, needle) {
			break
		}
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		t.Fatalf("read stream: %v", err)
	}
	return b.String()
}
