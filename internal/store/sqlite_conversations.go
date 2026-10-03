package store

import (
	"database/sql"
)

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
