package api

import (
	"testing"
	"time"

	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func TestCostFinalizerPersistsTerminalCost(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	profile, err := st.UpsertModelProfile(store.ModelProfile{Name: "finalizer", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := st.CreateRun(store.CreateRunInput{
		AgentID: "a1", Input: "x", ConversationID: "c1", ModelProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.AppendEvent(runRec.ID, store.Event{
		Type: run.EventLLMUsage,
		Data: map[string]any{
			"model_profile_id": profile.ID, "prompt_tokens": 1000, "completion_tokens": 1000, "total_tokens": 2000,
		},
	})
	if err := st.UpdateRun(runRec.ID, store.StatusSucceeded, "ok", ""); err != nil {
		t.Fatal(err)
	}
	clearAgentCostCache()
	clearConversationCostCache()
	n, err := srv.finalizeMissingRunCosts()
	if err != nil || n != 1 {
		t.Fatalf("finalized=%d err=%v", n, err)
	}
	cost, ok, err := st.GetRunCost(runRec.ID)
	if err != nil || !ok || !cost.Priced || cost.CostUSD <= 0 {
		t.Fatalf("cost=%+v ok=%v err=%v", cost, ok, err)
	}
	day, month, err := srv.agentCostTotals("a1", time.Now().UTC())
	if err != nil || day <= 0 || month < day {
		t.Fatalf("agent totals day=%v month=%v err=%v", day, month, err)
	}
	if again, err := srv.finalizeMissingRunCosts(); err != nil || again != 0 {
		t.Fatalf("repeat finalized=%d err=%v", again, err)
	}
}
