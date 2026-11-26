package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
)

const operatorLimitsKey = "operator_limits"

type operatorLimits struct {
	RequestsPerMinute int `json:"requests_per_minute"`
	RunsPerDay        int `json:"runs_per_day"`
	MaxConcurrentRuns int `json:"max_concurrent_runs"`
}

func (s *Server) operatorLimits() operatorLimits {
	if s.Store == nil {
		return operatorLimits{}
	}
	raw, ok, err := s.Store.GetSetting(operatorLimitsKey)
	if err != nil || !ok {
		return operatorLimits{}
	}
	var v operatorLimits
	_ = json.Unmarshal(raw, &v)
	return v
}

func (s *Server) handleGetOperatorLimits(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.operatorLimits())
}

func (s *Server) handlePutOperatorLimits(w http.ResponseWriter, r *http.Request) {
	var body operatorLimits
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_json", err.Error())
		return
	}
	if body.RequestsPerMinute < 0 || body.RequestsPerMinute > 100000 ||
		body.RunsPerDay < 0 || body.RunsPerDay > 100000 ||
		body.MaxConcurrentRuns < 0 || body.MaxConcurrentRuns > 10000 {
		writeError(w, http.StatusBadRequest, "invalid_request", "limits must be non-negative")
		return
	}
	raw, _ := json.Marshal(body)
	if err := s.Store.UpsertSetting(operatorLimitsKey, raw); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, body)
}

var operatorRateMu sync.Mutex
var operatorRateWindows = map[string]*rateWindow{}

type rateWindow struct {
	start time.Time
	count int
}

// allowOperatorRequest enforces the per-process request window. It is local to
// this replica; multi-replica deployments enforce per instance.
func (s *Server) allowOperatorRequest(ctx context.Context) bool {
	limits := s.operatorLimits()
	if limits.RequestsPerMinute <= 0 {
		return true
	}
	id := operatorLimitKey(ctx)
	if id == "" {
		return true
	}
	now := time.Now()
	operatorRateMu.Lock()
	defer operatorRateMu.Unlock()
	w := operatorRateWindows[id]
	if w == nil || now.Sub(w.start) >= time.Minute {
		w = &rateWindow{start: now}
		operatorRateWindows[id] = w
	}
	if w.count >= limits.RequestsPerMinute {
		return false
	}
	w.count++
	return true
}

func operatorLimitKey(ctx context.Context) string {
	if controlplane.RoleFrom(ctx) != controlplane.RoleOperator {
		return ""
	}
	if id := controlplane.OperatorIDFrom(ctx); id != "" {
		return id
	}
	return "operator"
}

func (s *Server) operatorConversationIDs(ctx context.Context) ([]string, error) {
	ms := s.metaStore()
	if ms == nil {
		return nil, nil
	}
	id := controlplane.OperatorIDFrom(ctx)
	if id == "" {
		return nil, nil
	}
	metas, err := ms.ListMeta(conversation.MetaFilter{OwnerID: id})
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(metas))
	for _, m := range metas {
		out = append(out, m.ID)
	}
	return out, nil
}

// checkOperatorRunLimits enforces daily run and concurrent-run quotas for the
// calling operator. Zero means unlimited. Admin and machine identities skip.
func (s *Server) checkOperatorRunLimits(ctx context.Context) error {
	if controlplane.RoleFrom(ctx) != controlplane.RoleOperator {
		return nil
	}
	limits := s.operatorLimits()
	if limits.RunsPerDay <= 0 && limits.MaxConcurrentRuns <= 0 {
		return nil
	}
	convs, err := s.operatorConversationIDs(ctx)
	if err != nil || len(convs) == 0 {
		return err
	}
	if limits.MaxConcurrentRuns > 0 {
		active := 0
		for _, id := range convs {
			busy, err := s.Store.HasActiveRun(id)
			if err != nil {
				return err
			}
			if busy {
				active++
				if active >= limits.MaxConcurrentRuns {
					return ErrOperatorConcurrency
				}
			}
		}
	}
	if limits.RunsPerDay > 0 {
		now := time.Now().UTC()
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		n, err := s.Store.CountRunsCreatedBetweenForConversations(convs, start, now)
		if err != nil {
			return err
		}
		if n >= limits.RunsPerDay {
			return ErrOperatorRunQuota
		}
	}
	return nil
}

var ErrOperatorRunQuota = errString("operator daily run quota exceeded")
var ErrOperatorConcurrency = errString("operator concurrent run limit exceeded")

type errString string

func (e errString) Error() string { return string(e) }
