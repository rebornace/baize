package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/run"
	"github.com/rebornace/baize/internal/store"
)

func seedWorkspaceUsage(t *testing.T, srv *Server, convID, wsID string) (store.ModelProfile, string) {
	t.Helper()
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	ws, err := ms.CreateWorkspace("Lab")
	if err != nil {
		t.Fatal(err)
	}
	if wsID != "" {
		ws = conversation.Workspace{ID: wsID, Name: "Lab"}
	}
	if err := ms.EnsureMeta(conversation.Meta{ID: convID, WorkspaceID: ws.ID, Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	profile, err := srv.Store.UpsertModelProfile(store.ModelProfile{Name: "ws-model", Model: "gpt-4o"})
	if err != nil {
		t.Fatal(err)
	}
	runRec, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: convID, ModelProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = srv.Store.AppendEvent(runRec.ID, store.Event{
		Type: run.EventLLMUsage,
		Data: map[string]any{
			"model_profile_id": profile.ID, "prompt_tokens": 1000, "completion_tokens": 1000, "total_tokens": 2000,
		},
	})
	clearWorkspaceCostCache()
	return profile, ws.ID
}

func TestWorkspaceCostBudgetsAPI(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	_, wsID := seedWorkspaceUsage(t, srv, "c-ws", "")
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/settings/workspace-cost-budgets",
		bytes.NewBufferString(`{"workspace_id":"`+wsID+
			`","budget_usd_per_day":1,"budget_usd_per_month":20,"warning_ratio":0.5,"exceeded_ratio":1.2}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	var saved workspaceBudgetJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Name != "Lab" || saved.BudgetDay != 1 || saved.SpentDay <= 0 || saved.SpentMonth <= 0 {
		t.Fatalf("saved=%+v", saved)
	}
	if d, m := srv.workspaceBudgets(wsID); d != 1 || m != 20 {
		t.Fatalf("budgets=%v/%v", d, m)
	}

	get := httptest.NewRequest(http.MethodGet, "/v0/settings/workspace-cost-budgets", nil)
	get.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, get)
	if rr.Code != http.StatusOK {
		t.Fatalf("get=%d %s", rr.Code, rr.Body.String())
	}
	var body struct {
		Items []workspaceBudgetJSON `json:"items"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, item := range body.Items {
		if item.WorkspaceID != wsID {
			continue
		}
		found = true
		if item.WarningRatio != 0.5 || item.ExceededRatio != 1.2 || item.SpentDay <= 0 {
			t.Fatalf("item=%+v", item)
		}
	}
	if !found {
		t.Fatalf("workspace missing from %+v", body.Items)
	}

	badThreshold := httptest.NewRequest(http.MethodPut, "/v0/settings/workspace-cost-budgets",
		bytes.NewBufferString(`{"workspace_id":"`+wsID+
			`","budget_usd_per_day":1,"warning_ratio":1.2,"exceeded_ratio":0.5}`))
	badThreshold.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, badThreshold)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("bad thresholds=%d", rr.Code)
	}

	missing := httptest.NewRequest(http.MethodPut, "/v0/settings/workspace-cost-budgets",
		bytes.NewBufferString(`{"workspace_id":"ws_nope","budget_usd_per_day":1}`))
	missing.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, missing)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("missing workspace=%d", rr.Code)
	}
}

func TestWorkspaceBudgetEnforcement(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	_, wsID := seedWorkspaceUsage(t, srv, "c-ws", "")
	if err := st.UpsertSetting(costBudgetEnforceKey, []byte("true")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(workspaceBudgetDayPrefix+wsID, []byte("0.005")); err != nil {
		t.Fatal(err)
	}
	clearWorkspaceCostCache()
	if err := srv.requireCostBudgetFor("a", "c-ws"); !errors.Is(err, ErrCostBudgetExceeded) {
		t.Fatalf("workspace err=%v, want ErrCostBudgetExceeded", err)
	}
	if err := srv.requireCostBudgetFor("a", "c-other"); err != nil {
		t.Fatalf("other conversation err=%v", err)
	}
}

func TestWorkspaceBudgetAlertState(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	st := srv.Store
	_, wsID := seedWorkspaceUsage(t, srv, "c-ws", "")
	if err := st.UpsertSetting(workspaceBudgetDayPrefix+wsID, []byte("0.005")); err != nil {
		t.Fatal(err)
	}
	if err := st.UpsertSetting(workspaceBudgetMonthPrefix+wsID, []byte("100")); err != nil {
		t.Fatal(err)
	}
	clearWorkspaceCostCache()
	if _, err := srv.checkBudgetAlerts(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, ok, err := st.GetSetting(costAlertStateKey)
	if err != nil || !ok {
		t.Fatalf("state missing ok=%v err=%v", ok, err)
	}
	var state budgetAlertState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	if got := state.Scopes["workspace:"+wsID]; got.DayLevel != "exceeded" {
		t.Fatalf("workspace scope=%+v all=%+v", got, state.Scopes)
	}
}
