package store

import (
	"strings"
	"time"
)

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

// CountRunsCreatedBetweenForConversations counts runs created in the window for
// an operator's conversations. At most 1000 ids are considered.
func (s *Memory) CountRunsCreatedBetweenForConversations(conversationIDs []string, from, to time.Time) (int, error) {
	allowed := make(map[string]bool, len(conversationIDs))
	for _, id := range conversationIDs {
		if id != "" {
			allowed[id] = true
		}
	}
	if len(allowed) == 0 {
		return 0, nil
	}
	from, to = from.UTC(), to.UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, r := range s.runs {
		if !allowed[r.ConversationID] {
			continue
		}
		if r.CreatedAt.Before(from) || r.CreatedAt.After(to) {
			continue
		}
		count++
	}
	return count, nil
}

func (s *SQLStore) CountRunsCreatedBetweenForConversations(conversationIDs []string, from, to time.Time) (int, error) {
	ids := make([]string, 0, len(conversationIDs))
	for _, id := range conversationIDs {
		if id != "" {
			ids = append(ids, id)
		}
		if len(ids) >= 1000 {
			break
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	args := make([]any, 0, len(ids)+2)
	args = append(args, formatSQLiteTime(from), formatSQLiteTime(to))
	for _, id := range ids {
		args = append(args, id)
	}
	var n int
	err := s.queryRow(`SELECT COUNT(*) FROM runs
		WHERE created_at >= ? AND created_at <= ? AND conversation_id IN (`+placeholders+`)`,
		args...).Scan(&n)
	if err != nil {
		return 0, err
	}
	return n, nil
}
