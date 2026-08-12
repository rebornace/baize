package integration_test

import (
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/config"
	"github.com/rebornace/baize/internal/bootstrap"
	"github.com/rebornace/baize/internal/store"
)

func TestHITLApproveCreatesTicket(t *testing.T) {
	cfg := hitlConfig(t, "")
	runtimeURL, ticketURL, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	before := ticketCount(t, ticketURL)
	runID := startRun(t, runtimeURL, "创建一个紧急工单：VPN 挂了")
	pollRun(t, runtimeURL, runID, store.StatusWaitingHuman)

	resumeRun(t, runtimeURL, runID, "approve", "ok")
	pollRun(t, runtimeURL, runID, store.StatusSucceeded)

	after := ticketCount(t, ticketURL)
	if after < before+1 {
		t.Fatalf("tickets after approve=%d before=%d want >= before+1", after, before)
	}
}

func TestHITLRejectNoTicket(t *testing.T) {
	cfg := hitlConfig(t, "")
	runtimeURL, ticketURL, shutdown := bootstrap.StartForTest(t, cfg)
	defer shutdown()

	before := ticketCount(t, ticketURL)
	runID := startRun(t, runtimeURL, "创建一个紧急工单：网络中断")
	pollRun(t, runtimeURL, runID, store.StatusWaitingHuman)

	resumeRun(t, runtimeURL, runID, "reject", "nope")
	pollRun(t, runtimeURL, runID, store.StatusFailed)

	after := ticketCount(t, ticketURL)
	if after != before {
		t.Fatalf("tickets after reject=%d before=%d want unchanged", after, before)
	}
}

// TestHITLSQLiteReopenResume：waiting_human 后关掉 Runtime，同进程重开同一 SQLite，再 resume（cold ContinueFromHITL）。
func TestHITLSQLiteReopenResume(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "baize.db")
	cfg := hitlConfig(t, dbPath)

	runtimeURL, _, shutdown := bootstrap.StartForTest(t, cfg)
	runID := startRun(t, runtimeURL, "创建一个紧急工单：重启后续跑")
	pollRun(t, runtimeURL, runID, store.StatusWaitingHuman)
	shutdown()

	// 新 Runtime + 新 mock-ticket；SQLite 中仍为 waiting_human，走 cold ContinueFromHITL。
	runtimeURL2, ticketURL2, shutdown2 := bootstrap.StartForTest(t, cfg)
	defer shutdown2()

	before := ticketCount(t, ticketURL2)
	resumeRun(t, runtimeURL2, runID, "approve", "after reopen")
	pollRun(t, runtimeURL2, runID, store.StatusSucceeded)

	after := ticketCount(t, ticketURL2)
	if after < before+1 {
		t.Fatalf("tickets after reopen resume=%d before=%d want >= before+1", after, before)
	}
}

func hitlConfig(t *testing.T, sqlitePath string) config.Config {
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
	if sqlitePath != "" {
		cfg.Store.Driver = "sqlite"
		cfg.Store.SQLitePath = sqlitePath
	}
	return cfg
}

func startRun(t *testing.T, runtimeURL, input string) string {
	t.Helper()
	body := `{"agent_id":"ticket-agent","input":` + jsonString(input) + `}`
	resp, err := http.Post(runtimeURL+"/v0/runs", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v0/runs: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /v0/runs status=%d body=%s", resp.StatusCode, raw)
	}
	var created struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatalf("decode run: %v body=%s", err, raw)
	}
	if created.RunID == "" {
		t.Fatalf("missing run_id: %s", raw)
	}
	return created.RunID
}

func resumeRun(t *testing.T, runtimeURL, runID, decision, comment string) {
	t.Helper()
	body := `{"decision":"` + decision + `","comment":` + jsonString(comment) + `}`
	resp, err := http.Post(runtimeURL+"/v0/runs/"+runID+"/resume", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST resume: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume status=%d body=%s", resp.StatusCode, raw)
	}
}

func ticketCount(t *testing.T, ticketURL string) int {
	t.Helper()
	resp, err := http.Get(ticketURL + "/tickets")
	if err != nil {
		t.Fatalf("GET tickets: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET tickets status=%d body=%s", resp.StatusCode, raw)
	}
	var tickets []map[string]any
	if err := json.Unmarshal(raw, &tickets); err != nil {
		t.Fatalf("decode tickets: %v body=%s", err, raw)
	}
	return len(tickets)
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
