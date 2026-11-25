package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestCostBreakdownAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	lab, err := ms.CreateWorkspace("Lab")
	if err != nil {
		t.Fatal(err)
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: "c1", WorkspaceID: lab.ID, Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	m1, err := st.UpsertModelProfile(store.ModelProfile{Name: "cheap", Model: "gpt-4o-mini"})
	if err != nil {
		t.Fatal(err)
	}
	m2, err := st.UpsertModelProfile(store.ModelProfile{Name: "power", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	mk := func(agentID, modelID string, cost float64) {
		r, err := st.CreateRun(store.CreateRunInput{
			AgentID: agentID, Input: "x", ConversationID: "c1", ModelProfileID: modelID,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := st.UpdateRun(r.ID, store.StatusSucceeded, "ok", ""); err != nil {
			t.Fatal(err)
		}
		if err := st.PutRunCost(store.RunCost{
			RunID: r.ID, Day: now.Format("2006-01-02"), CostUSD: cost, Priced: true,
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("a1", m1.ID, 1)
	mk("a2", m2.ID, 2)
	req := httptest.NewRequest(http.MethodGet, "/v0/settings/cost-breakdown?days=7", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Total float64 `json:"total_usd"`
		Model []struct {
			Key     string  `json:"key"`
			Name    string  `json:"name"`
			CostUSD float64 `json:"cost_usd"`
			Ratio   float64 `json:"ratio"`
		} `json:"by_model"`
		Agent []struct {
			Key     string  `json:"key"`
			CostUSD float64 `json:"cost_usd"`
		} `json:"by_agent"`
		Conversation []struct {
			Key     string  `json:"key"`
			CostUSD float64 `json:"cost_usd"`
		} `json:"by_conversation"`
		Workspace []struct {
			Key     string  `json:"key"`
			Name    string  `json:"name"`
			CostUSD float64 `json:"cost_usd"`
		} `json:"by_workspace"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Total != 3 || len(body.Model) != 2 || len(body.Agent) != 2 ||
		len(body.Conversation) != 1 || len(body.Workspace) != 1 {
		t.Fatalf("body=%+v", body)
	}
	if body.Model[0].Name != "power" || body.Model[0].CostUSD != 2 || body.Model[0].Ratio != 0.6667 {
		t.Fatalf("model[0]=%+v", body.Model[0])
	}
	if body.Agent[0].Key != "a2" || body.Agent[0].CostUSD != 2 {
		t.Fatalf("agent[0]=%+v", body.Agent[0])
	}
	if body.Conversation[0].Key != "c1" || body.Conversation[0].CostUSD != 3 {
		t.Fatalf("conversation[0]=%+v", body.Conversation[0])
	}
	if body.Workspace[0].Key != lab.ID || body.Workspace[0].Name != "Lab" || body.Workspace[0].CostUSD != 3 {
		t.Fatalf("workspace[0]=%+v", body.Workspace[0])
	}
}
