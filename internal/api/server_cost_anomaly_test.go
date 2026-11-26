package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/store"
)

func seedCostRun(t *testing.T, st store.Store, agentID, modelID, convID, day string, cost float64) {
	t.Helper()
	r, err := st.CreateRun(store.CreateRunInput{
		AgentID: agentID, Input: "x", ConversationID: convID, ModelProfileID: modelID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.PutRunCost(store.RunCost{
		RunID: r.ID, Day: day, CostUSD: cost, Priced: true,
	}); err != nil {
		t.Fatal(err)
	}
}

func seedCost(t *testing.T, st store.Store, convID, day string, cost float64) {
	t.Helper()
	seedCostRun(t, st, "a", "", convID, day, cost)
}

func getAnomalyHistory(t *testing.T, srv *Server) []costAnomalyHistoryEntry {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v0/settings/cost-anomalies", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("history=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Items []costAnomalyHistoryEntry `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Items
}

func TestCostAnomalyDetection(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	baselineDay := now.AddDate(0, 0, -3).Format("2006-01-02")
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "anomaly", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	seedCostRun(t, st, "a", profile.ID, "c-spike", baselineDay, 7) // 7-day average = $1.00
	seedCostRun(t, st, "a", profile.ID, "c-spike", today, 3)
	seedCost(t, st, "c-small", today, 0.2)

	if n, err := srv.checkCostAnomalies(t.Context()); err != nil {
		t.Fatalf("sent=%d err=%v", n, err)
	}
	items := getAnomalyHistory(t, srv)
	if len(items) != 3 {
		t.Fatalf("items=%+v", items)
	}
	var conv, agent, model bool
	for _, it := range items {
		switch it.Scope {
		case "global":
			t.Fatalf("global must be suppressed when narrower scopes fired: %+v", it)
		case "conversation":
			conv = true
			if it.ConversationID != "c-spike" || it.Ratio < 2 {
				t.Fatalf("conv=%+v", it)
			}
		case "agent":
			agent = true
			if it.AgentID != "a" || it.Ratio < 2 {
				t.Fatalf("agent=%+v", it)
			}
		case "model":
			model = true
			if it.ModelID != profile.ID || it.Ratio < 2 {
				t.Fatalf("model=%+v", it)
			}
		}
	}
	if !conv || !agent || !model {
		t.Fatalf("conv=%v agent=%v model=%v items=%+v", conv, agent, model, items)
	}

	// Same day: deduped, no new history entries.
	if _, err := srv.checkCostAnomalies(t.Context()); err != nil {
		t.Fatal(err)
	}
	if again := getAnomalyHistory(t, srv); len(again) != 3 {
		t.Fatalf("dedupe failed: %+v", again)
	}
}

func TestCostAnomalyGlobalBroadSpike(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	now := time.Now().UTC()
	today := now.Format("2006-01-02")
	baselineDay := now.AddDate(0, 0, -3).Format("2006-01-02")
	seedCostRun(t, st, "a-old", "", "c-base", baselineDay, 7)
	for i := 0; i < 10; i++ {
		seedCostRun(t, st, fmt.Sprintf("a-new-%d", i), "", fmt.Sprintf("c-new-%d", i), today, 1)
	}
	if _, err := srv.checkCostAnomalies(t.Context()); err != nil {
		t.Fatal(err)
	}
	items := getAnomalyHistory(t, srv)
	if len(items) != 1 || items[0].Scope != "global" {
		t.Fatalf("items=%+v", items)
	}
}

func TestCostAnomalySilenced(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	now := time.Now().UTC()
	seedCost(t, st, "c-spike", now.AddDate(0, 0, -2).Format("2006-01-02"), 7)
	seedCost(t, st, "c-spike", now.Format("2006-01-02"), 3)

	raw, _ := json.Marshal(costAlertSilence{Until: now.Add(time.Hour), Reason: "window"})
	if err := st.UpsertSetting(costAlertSilenceKey, raw); err != nil {
		t.Fatal(err)
	}
	if _, err := srv.checkCostAnomalies(t.Context()); err != nil {
		t.Fatal(err)
	}
	if items := getAnomalyHistory(t, srv); len(items) != 0 {
		t.Fatalf("history=%+v", items)
	}
}

func TestCostAnomalyBelowFloor(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	now := time.Now().UTC()
	// Baseline average $0.10 but today $0.90: above ratio, below the $1 floor.
	seedCost(t, st, "c-tiny", now.AddDate(0, 0, -1).Format("2006-01-02"), 0.7)
	seedCost(t, st, "c-tiny", now.Format("2006-01-02"), 0.9)
	if _, err := srv.checkCostAnomalies(t.Context()); err != nil {
		t.Fatal(err)
	}
	if items := getAnomalyHistory(t, srv); len(items) != 0 {
		t.Fatalf("history=%+v", items)
	}
}
