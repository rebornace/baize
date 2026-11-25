package store

import (
	"sort"
	"strings"
	"time"
)

// maxRetentionExclusions bounds the NOT IN list so the query stays within
// driver parameter limits when many conversations override the global policy.
const maxRetentionExclusions = 5000

// DeleteTerminalRunsBefore deletes terminal runs older than cutoff, plus their
// events, cached costs and HITL payloads. Active runs are never touched.
func (s *Memory) DeleteTerminalRunsBefore(cutoff time.Time, limit int) (int, error) {
	return s.deleteTerminalRunsBefore(cutoff, limit, "", nil)
}

func (s *Memory) DeleteTerminalRunsBeforeExcluding(
	cutoff time.Time, limit int, excludeConversationIDs []string,
) (int, error) {
	return s.deleteTerminalRunsBefore(cutoff, limit, "", excludeConversationIDs)
}

func (s *Memory) DeleteTerminalRunsBeforeForConversation(conversationID string, cutoff time.Time, limit int) (int, error) {
	return s.deleteTerminalRunsBefore(cutoff, limit, conversationID, nil)
}

func (s *Memory) deleteTerminalRunsBefore(
	cutoff time.Time, limit int, conversationID string, exclude []string,
) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	cutoff = cutoff.UTC()
	excluded := make(map[string]bool, len(exclude))
	for _, id := range exclude {
		if id != "" {
			excluded[id] = true
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
		if conversationID != "" && r.ConversationID != conversationID {
			continue
		}
		if excluded[r.ConversationID] {
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
	return s.deleteTerminalRunsBefore(cutoff, limit, "", nil)
}

func (s *SQLStore) DeleteTerminalRunsBeforeExcluding(
	cutoff time.Time, limit int, excludeConversationIDs []string,
) (int, error) {
	return s.deleteTerminalRunsBefore(cutoff, limit, "", excludeConversationIDs)
}

func (s *SQLStore) DeleteTerminalRunsBeforeForConversation(conversationID string, cutoff time.Time, limit int) (int, error) {
	return s.deleteTerminalRunsBefore(cutoff, limit, conversationID, nil)
}

func (s *SQLStore) deleteTerminalRunsBefore(
	cutoff time.Time, limit int, conversationID string, exclude []string,
) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	q := `SELECT id FROM runs
		WHERE status IN ('succeeded','failed','cancelled','rejected') AND created_at < ?`
	args := []any{formatSQLiteTime(cutoff)}
	if conversationID != "" {
		q += ` AND conversation_id = ?`
		args = append(args, conversationID)
	}
	if len(exclude) > 0 {
		if len(exclude) > maxRetentionExclusions {
			exclude = exclude[:maxRetentionExclusions]
		}
		q += ` AND (conversation_id IS NULL OR conversation_id NOT IN (` +
			strings.TrimSuffix(strings.Repeat("?,", len(exclude)), ",") + `))`
		for _, id := range exclude {
			args = append(args, id)
		}
	}
	q += ` ORDER BY created_at ASC LIMIT ?`
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
