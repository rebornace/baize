package api

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

const workspaceBudgetDayPrefix = "cost_budget_workspace_day:"
const workspaceBudgetMonthPrefix = "cost_budget_workspace_month:"
const workspaceBudgetWarnPrefix = "cost_budget_workspace_warn:"
const workspaceBudgetExceedPrefix = "cost_budget_workspace_exceed:"
const workspaceCostCacheTTL = 30 * time.Second

type workspaceCostEntry struct {
	at    time.Time
	day   float64
	month float64
}

var workspaceCostCache struct {
	mu      sync.Mutex
	entries map[string]workspaceCostEntry
}

func (s *Server) workspaceBudget(wsID string, month bool) float64 {
	if s.Store == nil || wsID == "" {
		return 0
	}
	key := workspaceBudgetDayPrefix
	if month {
		key = workspaceBudgetMonthPrefix
	}
	raw, ok, err := s.Store.GetSetting(key + wsID)
	if err != nil || !ok {
		return 0
	}
	var v float64
	if json.Unmarshal(raw, &v) != nil || v < 0 || v > 1_000_000 {
		return 0
	}
	return v
}

func (s *Server) workspaceBudgets(wsID string) (day, month float64) {
	return s.workspaceBudget(wsID, false), s.workspaceBudget(wsID, true)
}

func (s *Server) workspaceBudgetThresholds(wsID string) (warn, exceed float64) {
	return s.scopeRatio(workspaceBudgetWarnPrefix + wsID),
		s.scopeRatio(workspaceBudgetExceedPrefix + wsID)
}

func clearWorkspaceCostCache() {
	workspaceCostCache.mu.Lock()
	workspaceCostCache.entries = nil
	workspaceCostCache.mu.Unlock()
}

// workspaceConversationIDs returns the conversations that belong to a
// workspace. Conversations without metadata count toward the default workspace
// so API-created runs are not silently excluded.
func (s *Server) workspaceConversationIDs(wsID string) (map[string]bool, error) {
	out := map[string]bool{}
	ms := s.metaStore()
	if ms == nil {
		return out, nil
	}
	if wsID != conversation.DefaultWorkspaceID {
		list, err := ms.ListMeta(conversation.MetaFilter{WorkspaceID: wsID})
		if err != nil {
			return nil, err
		}
		for _, m := range list {
			out[m.ID] = true
		}
		return out, nil
	}
	all, err := ms.ListMeta(conversation.MetaFilter{})
	if err != nil {
		return nil, err
	}
	hasMeta := map[string]bool{}
	for _, m := range all {
		hasMeta[m.ID] = true
		if conversation.NormalizeWorkspaceID(m.WorkspaceID) == conversation.DefaultWorkspaceID {
			out[m.ID] = true
		}
	}
	if s.Store != nil {
		ids, err := s.Store.ListRunConversationIDs()
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if !hasMeta[id] {
				out[id] = true
			}
		}
	}
	return out, nil
}

// workspaceCostTotals sums terminal conversation costs for one workspace and
// adds recent in-flight usage. Results are cached briefly because run start
// checks the workspace budget on every new run.
func (s *Server) workspaceCostTotals(wsID string, now time.Time) (float64, float64, error) {
	if wsID == "" || s.Store == nil {
		return 0, 0, nil
	}
	workspaceCostCache.mu.Lock()
	if e, ok := workspaceCostCache.entries[wsID]; ok && now.Sub(e.at) < workspaceCostCacheTTL {
		workspaceCostCache.mu.Unlock()
		return e.day, e.month, nil
	}
	workspaceCostCache.mu.Unlock()

	convs, err := s.workspaceConversationIDs(wsID)
	if err != nil {
		return 0, 0, err
	}
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	dayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	monthByConv, err := s.Store.SumRunCostsGrouped("conversation", monthStart, now)
	if err != nil {
		return 0, 0, err
	}
	dayByConv, err := s.Store.SumRunCostsGrouped("conversation", dayStart, now)
	if err != nil {
		return 0, 0, err
	}
	day, month := 0.0, 0.0
	for convID := range convs {
		month += monthByConv[convID]
		day += dayByConv[convID]
	}
	runs, err := s.Store.ListRunsCreatedBetweenForScope(monthStart, now, "", "", scopePartialScanLimit)
	if err != nil {
		return 0, 0, err
	}
	for _, rec := range runs {
		if !convs[rec.ConversationID] || isTerminalRunStatus(rec.Status) {
			continue
		}
		cost := s.runCostCached(rec)
		month += cost
		if !rec.CreatedAt.Before(dayStart) {
			day += cost
		}
	}
	day, month = round6(day), round6(month)
	workspaceCostCache.mu.Lock()
	if workspaceCostCache.entries == nil {
		workspaceCostCache.entries = map[string]workspaceCostEntry{}
	}
	workspaceCostCache.entries[wsID] = workspaceCostEntry{at: now, day: day, month: month}
	workspaceCostCache.mu.Unlock()
	return day, month, nil
}

type workspaceBudgetJSON struct {
	WorkspaceID   string  `json:"workspace_id"`
	Name          string  `json:"name"`
	BudgetDay     float64 `json:"budget_usd_per_day"`
	BudgetMonth   float64 `json:"budget_usd_per_month"`
	WarningRatio  float64 `json:"warning_ratio"`
	ExceededRatio float64 `json:"exceeded_ratio"`
	SpentDay      float64 `json:"spent_usd_per_day"`
	SpentMonth    float64 `json:"spent_usd_per_month"`
}

func (s *Server) workspaceList() ([]conversation.Workspace, error) {
	if wst := s.workspaceStore(); wst != nil {
		list, err := wst.ListWorkspaces()
		if err != nil {
			return nil, err
		}
		if len(list) > 0 {
			return list, nil
		}
	}
	return []conversation.Workspace{{
		ID:   conversation.DefaultWorkspaceID,
		Name: conversation.DefaultWorkspaceName,
	}}, nil
}

func (s *Server) handleGetWorkspaceCostBudgets(w http.ResponseWriter, r *http.Request) {
	list, err := s.workspaceList()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	now := time.Now().UTC()
	items := make([]workspaceBudgetJSON, 0, len(list))
	for _, ws := range list {
		day, month := s.workspaceBudgets(ws.ID)
		warn, exceed := s.workspaceBudgetThresholds(ws.ID)
		item := workspaceBudgetJSON{
			WorkspaceID: ws.ID, Name: ws.Name,
			BudgetDay: day, BudgetMonth: month,
			WarningRatio: warn, ExceededRatio: exceed,
		}
		if day > 0 || month > 0 {
			if spentDay, spentMonth, cerr := s.workspaceCostTotals(ws.ID, now); cerr == nil {
				item.SpentDay, item.SpentMonth = spentDay, spentMonth
			}
		}
		items = append(items, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) handlePutWorkspaceCostBudget(w http.ResponseWriter, r *http.Request) {
	var body workspaceBudgetJSON
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	body.WorkspaceID = conversation.NormalizeWorkspaceID(body.WorkspaceID)
	if body.BudgetDay < 0 || body.BudgetDay > 1_000_000 ||
		body.BudgetMonth < 0 || body.BudgetMonth > 1_000_000 {
		writeError(w, http.StatusBadRequest, "invalid_request", "budgets must be between 0 and 1000000")
		return
	}
	if !s.scopeThresholdsValid(body.WarningRatio, body.ExceededRatio) {
		writeError(w, http.StatusBadRequest, "invalid_request",
			"thresholds must be 0 (inherit) or in (0,5], with warning below exceeded")
		return
	}
	if wst := s.workspaceStore(); wst != nil {
		ws, err := wst.GetWorkspace(body.WorkspaceID)
		if err != nil {
			writeError(w, http.StatusNotFound, "not_found", "workspace not found")
			return
		}
		body.Name = ws.Name
	} else {
		body.Name = conversation.DefaultWorkspaceName
	}
	dayRaw, _ := json.Marshal(body.BudgetDay)
	monthRaw, _ := json.Marshal(body.BudgetMonth)
	warnRaw, _ := json.Marshal(body.WarningRatio)
	exceedRaw, _ := json.Marshal(body.ExceededRatio)
	for key, raw := range map[string][]byte{
		workspaceBudgetDayPrefix + body.WorkspaceID:    dayRaw,
		workspaceBudgetMonthPrefix + body.WorkspaceID:  monthRaw,
		workspaceBudgetWarnPrefix + body.WorkspaceID:   warnRaw,
		workspaceBudgetExceedPrefix + body.WorkspaceID: exceedRaw,
	} {
		if err := s.Store.UpsertSetting(key, raw); err != nil {
			writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
			return
		}
	}
	clearWorkspaceCostCache()
	s.resetScopeAlertState("workspace:" + body.WorkspaceID)
	if body.BudgetDay > 0 || body.BudgetMonth > 0 {
		if spentDay, spentMonth, err := s.workspaceCostTotals(body.WorkspaceID, time.Now().UTC()); err == nil {
			body.SpentDay, body.SpentMonth = spentDay, spentMonth
		}
	}
	writeJSON(w, http.StatusOK, body)
}
