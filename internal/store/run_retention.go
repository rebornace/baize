package store

import (
	"sort"
	"strings"
	"time"
)

// DeleteTerminalRunsBefore deletes terminal runs older than cutoff, plus their
// events, cached costs and HITL payloads. Active runs are never touched.
func (s *Memory) DeleteTerminalRunsBefore(cutoff time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	cutoff = cutoff.UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	type candidate struct {
		id   string
		when time.Time
	}
	var candidates []candidate
	for id, r := range s.runs {
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

func (s *SQLStore) DeleteTerminalRunsBefore(cutoff time.Time, limit int) (int, error) {
	if limit <= 0 {
		limit = 500
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(s.q(`SELECT id FROM runs
		WHERE status IN ('succeeded','failed','cancelled','rejected') AND created_at < ?
		ORDER BY created_at ASC LIMIT ?`), formatSQLiteTime(cutoff), limit)
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
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	for _, q := range []string{
		`DELETE FROM events WHERE run_id IN (` + placeholders + `)`,
		`DELETE FROM run_costs WHERE run_id IN (` + placeholders + `)`,
	} {
		if _, err := tx.Exec(s.q(q), args...); err != nil {
			return 0, err
		}
	}
	res, err := tx.Exec(s.q(`DELETE FROM runs WHERE id IN (`+placeholders+`)`), args...)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func terminalRunStatus(st Status) bool {
	switch st {
	case StatusSucceeded, StatusFailed, StatusCancelled, StatusRejected:
		return true
	}
	return false
}
