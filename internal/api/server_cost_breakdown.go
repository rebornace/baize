package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

type costBucketsJSON struct {
	Key     string  `json:"key"`
	Name    string  `json:"name"`
	CostUSD float64 `json:"cost_usd"`
	Ratio   float64 `json:"ratio"`
}

// costGroups holds terminal costs split by the dimensions shown on the cost
// page and in the breakdown export.
type costGroups struct {
	model        map[string]float64
	agent        map[string]float64
	conversation map[string]float64
}

// costBreakdownTopConversations bounds the conversation list so the settings
// response stays small when many conversations have usage.
const costBreakdownTopConversations = 20
const costBreakdownTopWorkspaces = 20

// costGroupsBetween sums cached terminal costs for the three cost dimensions.
func (s *Server) costGroupsBetween(from, to time.Time) (costGroups, error) {
	var g costGroups
	var err error
	if g.model, err = s.Store.SumRunCostsGrouped("model", from, to); err != nil {
		return g, err
	}
	if g.agent, err = s.Store.SumRunCostsGrouped("agent", from, to); err != nil {
		return g, err
	}
	if g.conversation, err = s.Store.SumRunCostsGrouped("conversation", from, to); err != nil {
		return g, err
	}
	return g, nil
}

// costGroupNames resolves display names for models, agents and conversations.
func (s *Server) costGroupNames() (model, agent, conversation map[string]string) {
	model = map[string]string{}
	if profiles, err := s.Store.ListModelProfiles(); err == nil {
		for _, p := range profiles {
			model[p.ID] = p.Name
		}
	}
	agent = map[string]string{}
	for _, ag := range s.Store.ListAgents() {
		agent[ag.ID] = ag.ID
	}
	conversation = map[string]string{}
	if s.Messages != nil {
		for _, sum := range s.Messages.ListSummaries() {
			conversation[sum.ID] = sum.Title
		}
	}
	return model, agent, conversation
}

// costByWorkspace rolls conversation costs up to workspaces. Conversations
// without metadata fall into the default workspace.
func (s *Server) costByWorkspace(byConversation map[string]float64) (map[string]float64, map[string]string) {
	convWorkspace := map[string]string{}
	if ms := s.metaStore(); ms != nil {
		if list, err := ms.ListMeta(conversation.MetaFilter{}); err == nil {
			for _, m := range list {
				convWorkspace[m.ID] = conversation.NormalizeWorkspaceID(m.WorkspaceID)
			}
		}
	}
	names := map[string]string{}
	if wst := s.workspaceStore(); wst != nil {
		if list, err := wst.ListWorkspaces(); err == nil {
			for _, w := range list {
				names[w.ID] = w.Name
			}
		}
	}
	out := map[string]float64{}
	for convID, cost := range byConversation {
		wsID := convWorkspace[convID]
		if wsID == "" {
			wsID = conversation.DefaultWorkspaceID
		}
		out[wsID] += cost
	}
	return out, names
}

// costByProvider rolls model costs up to their provider string.
func (s *Server) costByProvider(byModel map[string]float64) (map[string]float64, map[string]string) {
	providerByModel := map[string]string{}
	if profiles, err := s.Store.ListModelProfiles(); err == nil {
		for _, p := range profiles {
			providerByModel[p.ID] = p.Provider
		}
	}
	out := map[string]float64{}
	names := map[string]string{}
	for modelID, cost := range byModel {
		provider := strings.TrimSpace(providerByModel[modelID])
		if provider == "" {
			provider = "unknown"
		}
		out[provider] += cost
		names[provider] = provider
	}
	return out, names
}

// handleGetCostBreakdown returns cached terminal costs grouped by model, agent
// and conversation for the last N days, newest range first.
func (s *Server) handleGetCostBreakdown(w http.ResponseWriter, r *http.Request) {
	days := 30
	if raw := r.URL.Query().Get("days"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil {
			days = n
		}
	}
	if days < 1 {
		days = 1
	}
	if days > 365 {
		days = 365
	}
	now := time.Now().UTC()
	from := now.AddDate(0, 0, -days)
	groups, err := s.costGroupsBetween(from, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	modelNames, agentNames, convNames := s.costGroupNames()
	byWorkspace, workspaceNames := s.costByWorkspace(groups.conversation)
	byProvider, providerNames := s.costByProvider(groups.model)
	total := 0.0
	for _, v := range groups.model {
		total += v
	}
	convBuckets := buildCostBuckets(groups.conversation, convNames, total)
	if len(convBuckets) > costBreakdownTopConversations {
		convBuckets = convBuckets[:costBreakdownTopConversations]
	}
	wsBuckets := buildCostBuckets(byWorkspace, workspaceNames, total)
	if len(wsBuckets) > costBreakdownTopWorkspaces {
		wsBuckets = wsBuckets[:costBreakdownTopWorkspaces]
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"days":            days,
		"from":            from.Format(time.RFC3339),
		"to":              now.Format(time.RFC3339),
		"total_usd":       round6(total),
		"by_model":        buildCostBuckets(groups.model, modelNames, total),
		"by_provider":     buildCostBuckets(byProvider, providerNames, total),
		"by_agent":        buildCostBuckets(groups.agent, agentNames, total),
		"by_conversation": convBuckets,
		"by_workspace":    wsBuckets,
	})
}

func buildCostBuckets(sums map[string]float64, names map[string]string, total float64) []costBucketsJSON {
	items := make([]costBucketsJSON, 0, len(sums))
	for key, cost := range sums {
		ratio := 0.0
		if total > 0 {
			ratio = cost / total
		}
		name := names[key]
		items = append(items, costBucketsJSON{
			Key: key, Name: name, CostUSD: round6(cost), Ratio: round4(ratio),
		})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CostUSD == items[j].CostUSD {
			return items[i].Key < items[j].Key
		}
		return items[i].CostUSD > items[j].CostUSD
	})
	return items
}
