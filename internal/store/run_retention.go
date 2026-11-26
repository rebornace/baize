package store

import (
	"sort"
	"strings"
	"time"
)

// maxRetentionExclusions bounds the NOT IN lists so the queries stay within
// driver parameter limits when many overrides exist.
const maxRetentionExclusions = 5000

// runDeleteFilter selects terminal runs for retention cleanup.
type runDeleteFilter struct {
	conversationID       string
	agentID              string
	excludeConversations []string
	excludeAgents        []string
}

// DeleteTerminalRunsBefore deletes terminal runs older than cutoff, plus their
// events, cached costs and HITL payloads. Active runs are never touched.
func (s *Memory) DeleteTerminalRunsBefore(cutoff time.Time, limit int) (int, error) {
	return s.deleteTerminalRunsBefore(runDeleteFilter{}, cutoff, limit)
}

func (s *Memory) DeleteTerminalRunsBeforeExcluding(
	cutoff time.Time, limit int, excludeConversationIDs, excludeAgentIDs []string,
) (int, error) {
	return s.deleteTerminalRunsBefore(runDeleteFilter{
		excludeConversations: excludeConversationIDs,
		excludeAgents:        excludeAgentIDs,
	}, cutoff, limit)
}

func (s *Memory) DeleteTerminalRunsBeforeForConversation(conversationID string, cutoff time.Time, limit int) (int, error) {
	return s.deleteTerminalRunsBefore(runDeleteFilter{conversationID: conversationID}, cutoff, limit)
}

func (s *Memory) DeleteTerminalRunsBeforeForAgent(
	agentID string, cutoff time.Time, limit int, excludeConversationIDs []string,
) (int, error) {
	return s.deleteTerminalRunsBefore(runDeleteFilter{
		agentID:              agentID,
		excludeConversations: excludeConversationIDs,
	}, cutoff, limit)
}

func (s *Memory) deleteTerminalRunsBefore(
	filter runDeleteFilter, cutoff time.Time, limit int,
) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	cutoff = cutoff.UTC()
	excludedConv := make(map[string]bool, len(filter.excludeConversations))
	for _, id := range filter.excludeConversations {
		if id != "" {
			excludedConv[id] = true
		}
	}
	excludedAgent := make(map[string]bool, len(filter.excludeAgents))
	for _, id := range filter.excludeAgents {
		if id != "" {
			excludedAgent[id] = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	type candidate struct {
		id   string
		when time.Time
	}
	var candidates []candidate
	for id, r := range s.runs {
		if filter.conversationID != "" && r.ConversationID != filter.conversationID {
			continue
		}
		if filter.agentID != "" && r.AgentID != filter.agentID {
			continue
		}
		if excludedConv[r.ConversationID] || excludedAgent[r.AgentID] {
			continue
		}
		if !terminalRunStatus(r.Status) || !r.CreatedAt.Before(cutoff) {
			continue
		}
		candidates = append(candidates, candidate{id: id, when: r.CreatedAt})
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].when.Before(candidates[j].when)
	})
	if len(candidates) > limit {
		candidates = candidates[:limit]
	}
	for _, c := range candidates {
		delete(s.runs, c.id)
		delete(s.events, c.id)
		delete(s.runCosts, c.id)
		delete(s.hitl, c.id)
	}
	return len(candidates), nil
}

func (s *Memory) CountTerminalRunsBeforeExcluding(
	cutoff time.Time, excludeConversationIDs, excludeAgentIDs []string,
) (int, error) {
	return s.countTerminalRunsBefore(runDeleteFilter{
		excludeConversations: excludeConversationIDs,
		excludeAgents:        excludeAgentIDs,
	}, cutoff), nil
}

func (s *Memory) CountTerminalRunsBeforeForConversation(conversationID string, cutoff time.Time) (int, error) {
	return s.countTerminalRunsBefore(runDeleteFilter{conversationID: conversationID}, cutoff), nil
}

func (s *Memory) CountTerminalRunsBeforeForAgent(
	agentID string, cutoff time.Time, excludeConversationIDs []string,
) (int, error) {
	return s.countTerminalRunsBefore(runDeleteFilter{
		agentID:              agentID,
		excludeConversations: excludeConversationIDs,
	}, cutoff), nil
}

func (s *Memory) countTerminalRunsBefore(filter runDeleteFilter, cutoff time.Time) int {
	cutoff = cutoff.UTC()
	excludedConv := make(map[string]bool, len(filter.excludeConversations))
	for _, id := range filter.excludeConversations {
		if id != "" {
			excludedConv[id] = true
		}
	}
	excludedAgent := make(map[string]bool, len(filter.excludeAgents))
	for _, id := range filter.excludeAgents {
		if id != "" {
			excludedAgent[id] = true
		}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	count := 0
	for _, r := range s.runs {
		if filter.conversationID != "" && r.ConversationID != filter.conversationID {
			continue
		}
		if filter.agentID != "" && r.AgentID != filter.agentID {
			continue
		}
		if excludedConv[r.ConversationID] || excludedAgent[r.AgentID] {
			continue
		}
		if terminalRunStatus(r.Status) && r.CreatedAt.Before(cutoff) {
			count++
		}
	}
	return count
}

func (s *Memory) ListRunConversationIDs() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, r := range s.runs {
		if r.ConversationID != "" {
			seen[r.ConversationID] = true
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

func (s *SQLStore) DeleteTerminalRunsBefore(cutoff time.Time, limit int) (int, error) {
	return s.deleteTerminalRunsBefore(runDeleteFilter{}, cutoff, limit)
}

func (s *SQLStore) DeleteTerminalRunsBeforeExcluding(
	cutoff time.Time, limit int, excludeConversationIDs, excludeAgentIDs []string,
) (int, error) {
	return s.deleteTerminalRunsBefore(runDeleteFilter{
		excludeConversations: excludeConversationIDs,
		excludeAgents:        excludeAgentIDs,
	}, cutoff, limit)
}

func (s *SQLStore) DeleteTerminalRunsBeforeForConversation(conversationID string, cutoff time.Time, limit int) (int, error) {
	return s.deleteTerminalRunsBefore(runDeleteFilter{conversationID: conversationID}, cutoff, limit)
}

func (s *SQLStore) DeleteTerminalRunsBeforeForAgent(
	agentID string, cutoff time.Time, limit int, excludeConversationIDs []string,
) (int, error) {
	return s.deleteTerminalRunsBefore(runDeleteFilter{
		agentID:              agentID,
		excludeConversations: excludeConversationIDs,
	}, cutoff, limit)
}

func (s *SQLStore) CountTerminalRunsBeforeExcluding(
	cutoff time.Time, excludeConversationIDs, excludeAgentIDs []string,
) (int, error) {
	return s.countTerminalRunsBefore(runDeleteFilter{
		excludeConversations: excludeConversationIDs,
		excludeAgents:        excludeAgentIDs,
	}, cutoff)
}

func (s *SQLStore) CountTerminalRunsBeforeForConversation(conversationID string, cutoff time.Time) (int, error) {
	return s.countTerminalRunsBefore(runDeleteFilter{conversationID: conversationID}, cutoff)
}

func (s *SQLStore) CountTerminalRunsBeforeForAgent(
	agentID string, cutoff time.Time, excludeConversationIDs []string,
) (int, error) {
	return s.countTerminalRunsBefore(runDeleteFilter{
		agentID:              agentID,
		excludeConversations: excludeConversationIDs,
	}, cutoff)
}

func (s *SQLStore) countTerminalRunsBefore(filter runDeleteFilter, cutoff time.Time) (int, error) {
	where, args := s.runRetentionWhere(filter, cutoff)
	var n int
	if err := s.queryRow(`SELECT COUNT(*) FROM runs `+where, args...).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

// runRetentionWhere builds the shared predicate for delete and preview count.
func (s *SQLStore) runRetentionWhere(filter runDeleteFilter, cutoff time.Time) (string, []any) {
	q := `WHERE status IN ('succeeded','failed','cancelled','rejected') AND created_at < ?`
	args := []any{formatSQLiteTime(cutoff)}
	if filter.conversationID != "" {
		q += ` AND conversation_id = ?`
		args = append(args, filter.conversationID)
	}
	if filter.agentID != "" {
		q += ` AND agent_id = ?`
		args = append(args, filter.agentID)
	}
	if len(filter.excludeConversations) > 0 {
		exclude := filter.excludeConversations
		if len(exclude) > maxRetentionExclusions {
			exclude = exclude[:maxRetentionExclusions]
		}
		q += ` AND (conversation_id IS NULL OR conversation_id NOT IN (` +
			strings.TrimSuffix(strings.Repeat("?,", len(exclude)), ",") + `))`
		for _, id := range exclude {
			args = append(args, id)
		}
	}
	if len(filter.excludeAgents) > 0 {
		exclude := filter.excludeAgents
		if len(exclude) > maxRetentionExclusions {
			exclude = exclude[:maxRetentionExclusions]
		}
		q += ` AND (agent_id IS NULL OR agent_id NOT IN (` +
			strings.TrimSuffix(strings.Repeat("?,", len(exclude)), ",") + `))`
		for _, id := range exclude {
			args = append(args, id)
		}
	}
	return q, args
}

func (s *SQLStore) deleteTerminalRunsBefore(
	filter runDeleteFilter, cutoff time.Time, limit int,
) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	where, args := s.runRetentionWhere(filter, cutoff)
	q := `SELECT id FROM runs ` + where + ` ORDER BY created_at ASC LIMIT ?`
	args = append(args, limit)
	rows, err := tx.Query(s.q(q), args...)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, tx.Commit()
	}

	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(ids)), ",")
	idArgs := make([]any, 0, len(ids))
	for _, id := range ids {
		idArgs = append(idArgs, id)
	}
	for _, q := range []string{
		`DELETE FROM events WHERE run_id IN (` + placeholders + `)`,
		`DELETE FROM run_costs WHERE run_id IN (` + placeholders + `)`,
	} {
		if _, err := tx.Exec(s.q(q), idArgs...); err != nil {
			return 0, err
		}
	}
	res, err := tx.Exec(s.q(`DELETE FROM runs WHERE id IN (`+placeholders+`)`), idArgs...)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (s *SQLStore) ListRunConversationIDs() ([]string, error) {
	rows, err := s.query(`SELECT DISTINCT conversation_id FROM runs
		WHERE conversation_id IS NOT NULL AND conversation_id <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	sort.Strings(out)
	return out, rows.Err()
}

func terminalRunStatus(st Status) bool {
	switch st {
	case StatusSucceeded, StatusFailed, StatusCancelled, StatusRejected:
		return true
	}
	return false
}
