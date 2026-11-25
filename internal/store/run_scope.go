package store

import "time"

// scopeRunsMaxLimit caps scoped run listings. Budgets for one agent or one
// conversation rarely need more than this many runs per month.
const scopeRunsMaxLimit = 5000

// ListRunsCreatedBetweenForScope returns runs in [from, to] for one agent or
// one conversation, newest first. Empty filters are ignored. At most limit
// rows are returned (cap 5000).
func (s *Memory) ListRunsCreatedBetweenForScope(from, to time.Time, agentID, conversationID string, limit int) ([]*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = scopeRunsMaxLimit
	}
	if limit > scopeRunsMaxLimit {
		limit = scopeRunsMaxLimit
	}
	from = from.UTC()
	to = to.UTC()
	var out []*Run
	for _, r := range s.runs {
		created := r.CreatedAt.UTC()
		if created.Before(from) || created.After(to) {
			continue
		}
		if agentID != "" && r.AgentID != agentID {
			continue
		}
		if conversationID != "" && r.ConversationID != conversationID {
			continue
		}
		cp := *r
		cp.PassthroughHeaders = cloneHeaders(r.PassthroughHeaders)
		cp.WebhookConfig = cloneWebhookConfig(r.WebhookConfig)
		if until, held := s.leaseUntil[r.ID]; held {
			lease := until
			cp.LeaseUntil = &lease
		}
		out = append(out, &cp)
	}
	sortRunsByCreatedAtDesc(out)
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *SQLStore) ListRunsCreatedBetweenForScope(from, to time.Time, agentID, conversationID string, limit int) ([]*Run, error) {
	if limit <= 0 {
		limit = scopeRunsMaxLimit
	}
	if limit > scopeRunsMaxLimit {
		limit = scopeRunsMaxLimit
	}
	from = from.UTC()
	to = to.UTC()
	q := `SELECT ` + runSelectColumns + ` FROM runs WHERE created_at >= ? AND created_at <= ?`
	args := []any{formatSQLiteTime(from), formatSQLiteTime(to)}
	if agentID != "" {
		q += ` AND agent_id = ?`
		args = append(args, agentID)
	}
	if conversationID != "" {
		q += ` AND conversation_id = ?`
		args = append(args, conversationID)
	}
	q += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(q, args...)
	if err != nil {
		return nil, err
	}
	return scanRuns(rows)
}
