package store

import (
	"database/sql"
	"fmt"
	"time"
)

func (s *SQLStore) PutChannelOutboxIfAbsent(entry ChannelOutboxEntry) (bool, string, error) {
	now := time.Now().UTC()
	normalizeChannelOutboxEntry(&entry, now)

	var existingID, existingStatus sql.NullString
	err := s.queryRow(
		`SELECT id, status FROM channel_outbox WHERE delivery_key = ?`, entry.DeliveryKey,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		if channelOutboxActive(ChannelOutboxStatus(existingStatus.String)) {
			return false, existingID.String, nil
		}
	} else if err != sql.ErrNoRows {
		return false, "", err
	}

	_, err = s.exec(`INSERT INTO channel_outbox (
		id, delivery_key, channel, kind, peer_id, conversation_id, account, run_id,
		payload_json, blob_keys_json, target_url,
		attempt, max_attempts, status, last_error, next_retry_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.DeliveryKey, entry.Channel, string(entry.Kind),
		entry.PeerID, entry.ConversationID, entry.Account, entry.RunID,
		string(entry.PayloadJSON), string(entry.BlobKeysJSON), entry.TargetURL,
		entry.Attempt, entry.MaxAttempts, string(entry.Status), entry.LastError,
		entry.NextRetryAt.UTC().Format(time.RFC3339Nano),
		entry.CreatedAt.UTC().Format(time.RFC3339Nano),
		entry.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, "", err
	}
	return true, entry.ID, nil
}

func scanChannelOutboxRow(sc interface {
	Scan(dest ...any) error
}) (ChannelOutboxEntry, error) {
	var e ChannelOutboxEntry
	var kind, status, payload, blobKeys, nextRetry, created, updated string
	var peerID, conversationID, account, runID, lastError sql.NullString
	if err := sc.Scan(
		&e.ID, &e.DeliveryKey, &e.Channel, &kind,
		&peerID, &conversationID, &account, &runID,
		&payload, &blobKeys, &e.TargetURL,
		&e.Attempt, &e.MaxAttempts, &status, &lastError, &nextRetry, &created, &updated,
	); err != nil {
		return ChannelOutboxEntry{}, err
	}
	e.Kind = ChannelOutboxKind(kind)
	e.Status = ChannelOutboxStatus(status)
	e.PayloadJSON = []byte(payload)
	e.BlobKeysJSON = []byte(blobKeys)
	if peerID.Valid {
		e.PeerID = peerID.String
	}
	if conversationID.Valid {
		e.ConversationID = conversationID.String
	}
	if account.Valid {
		e.Account = account.String
	}
	if runID.Valid {
		e.RunID = runID.String
	}
	if lastError.Valid {
		e.LastError = lastError.String
	}
	var err error
	e.NextRetryAt, err = time.Parse(time.RFC3339Nano, nextRetry)
	if err != nil {
		return ChannelOutboxEntry{}, fmt.Errorf("parse next_retry_at: %w", err)
	}
	e.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return ChannelOutboxEntry{}, fmt.Errorf("parse created_at: %w", err)
	}
	e.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return ChannelOutboxEntry{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return e, nil
}

const channelOutboxSelectColumns = `id, delivery_key, channel, kind, peer_id, conversation_id, account, run_id,
		payload_json, blob_keys_json, target_url,
		attempt, max_attempts, status, last_error, next_retry_at, created_at, updated_at`

func (s *SQLStore) ListChannelOutboxDue(now time.Time, limit int) ([]ChannelOutboxEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.query(fmt.Sprintf(`
		SELECT %s
		FROM channel_outbox
		WHERE status = ? AND next_retry_at <= ?
		ORDER BY next_retry_at ASC, created_at ASC
		LIMIT ?`, channelOutboxSelectColumns),
		string(ChannelOutboxPending), now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChannelOutboxEntry
	for rows.Next() {
		e, err := scanChannelOutboxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLStore) ListChannelOutbox(channel string, statuses []ChannelOutboxStatus, limit int) ([]ChannelOutboxEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	args := []any{channel}
	q := fmt.Sprintf(`
		SELECT %s
		FROM channel_outbox
		WHERE channel = ?`, channelOutboxSelectColumns)
	if len(statuses) > 0 {
		placeholders := make([]string, len(statuses))
		for i, st := range statuses {
			placeholders[i] = "?"
			args = append(args, string(st))
		}
		q += fmt.Sprintf(` AND status IN (%s)`, joinPlaceholders(placeholders))
	}
	q += ` ORDER BY updated_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ChannelOutboxEntry
	for rows.Next() {
		e, err := scanChannelOutboxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetChannelOutbox(id string) (ChannelOutboxEntry, error) {
	row := s.queryRow(fmt.Sprintf(`
		SELECT %s
		FROM channel_outbox WHERE id = ?`, channelOutboxSelectColumns), id)
	e, err := scanChannelOutboxRow(row)
	if err == sql.ErrNoRows {
		return ChannelOutboxEntry{}, ErrChannelOutboxNotFound
	}
	return e, err
}

func (s *SQLStore) UpdateChannelOutbox(entry ChannelOutboxEntry) error {
	entry.UpdatedAt = time.Now().UTC()
	res, err := s.exec(`UPDATE channel_outbox SET
		attempt = ?, max_attempts = ?, status = ?, last_error = ?, next_retry_at = ?, updated_at = ?
		WHERE id = ?`,
		entry.Attempt, entry.MaxAttempts, string(entry.Status), entry.LastError,
		entry.NextRetryAt.UTC().Format(time.RFC3339Nano),
		entry.UpdatedAt.UTC().Format(time.RFC3339Nano),
		entry.ID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrChannelOutboxNotFound
	}
	return nil
}

func (s *SQLStore) ResetChannelOutboxRetry(id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.exec(`UPDATE channel_outbox SET
		status = ?, attempt = 0, last_error = '', next_retry_at = ?, updated_at = ?
		WHERE id = ?`,
		string(ChannelOutboxPending), now, now, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrChannelOutboxNotFound
	}
	return nil
}
