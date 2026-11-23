package store

import (
	"database/sql"
	"time"
)

func (s *SQLStore) ListRunsByConversationSince(conversationID string, since time.Time, limit int) ([]*Run, error) {
	if conversationID == "" {
		return nil, nil
	}
	limit = normalizeConversationRunsLimit(limit)
	since = since.UTC()
	rows, err := s.query(
		`SELECT `+runSelectColumns+` FROM runs WHERE conversation_id = ? AND created_at >= ? ORDER BY created_at ASC LIMIT ?`,
		conversationID, s.timeArg(since), limit,
	)
	if err != nil {
		return nil, err
	}
	return scanRuns(rows)
}

func (s *SQLStore) LatestConversationRun(conversationID string) (*Run, error) {
	if conversationID == "" {
		return nil, ErrNoPriorConversationRun
	}
	var id string
	err := s.queryRow(
		`SELECT id FROM runs WHERE conversation_id = ? ORDER BY created_at DESC LIMIT 1`,
		conversationID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, ErrNoPriorConversationRun
	}
	if err != nil {
		return nil, err
	}
	return s.GetRun(id)
}

func (s *SQLStore) HasActiveRun(conversationID string) (bool, error) {
	if conversationID == "" {
		return false, nil
	}
	var one int
	err := s.queryRow(
		`SELECT 1 FROM runs WHERE conversation_id = ? AND status IN ('queued','running','waiting_human') LIMIT 1`,
		conversationID,
	).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *SQLStore) WaitingHumanRun(conversationID string) (*Run, error) {
	if conversationID == "" {
		return nil, nil
	}
	var id string
	err := s.queryRow(
		`SELECT id FROM runs WHERE conversation_id = ? AND status = ? ORDER BY created_at DESC LIMIT 1`,
		conversationID, string(StatusWaitingHuman),
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s.GetRun(id)
}
