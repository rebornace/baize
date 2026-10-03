package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

func (s *SQLStore) getInboxDeliveryRaw(channelID, idempotencyKey string) (InboxDelivery, bool, error) {
	var d InboxDelivery
	var createdAt string
	err := s.queryRow(
		`SELECT channel_id, idempotency_key, delivery_id, run_id, body_hash, created_at
		 FROM inbox_deliveries WHERE channel_id = ? AND idempotency_key = ?`,
		channelID, idempotencyKey,
	).Scan(&d.ChannelID, &d.IdempotencyKey, &d.DeliveryID, &d.RunID, &d.BodyHash, &createdAt)
	if err == sql.ErrNoRows {
		return InboxDelivery{}, false, nil
	}
	if err != nil {
		return InboxDelivery{}, false, err
	}
	ts, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return InboxDelivery{}, false, fmt.Errorf("parse created_at: %w", err)
		}
	}
	d.CreatedAt = ts
	return d, true, nil
}

func (s *SQLStore) GetInboxDelivery(channelID, idempotencyKey string) (InboxDelivery, bool, error) {
	d, ok, err := s.getInboxDeliveryRaw(channelID, idempotencyKey)
	if err != nil || !ok {
		return d, ok, err
	}
	// Rows older than InboxDeliveryTTL are treated as misses so the same key
	// may be claimed again after the window.
	if !InboxDeliveryFresh(d, time.Now()) {
		return InboxDelivery{}, false, nil
	}
	return d, true, nil
}

func (s *SQLStore) PutInboxDelivery(d InboxDelivery) error {
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	_, err := s.exec(
		`INSERT INTO inbox_deliveries (channel_id, idempotency_key, delivery_id, run_id, body_hash, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		d.ChannelID, d.IdempotencyKey, d.DeliveryID, d.RunID, d.BodyHash,
		d.CreatedAt.Format(time.RFC3339Nano),
	)
	if err == nil {
		return nil
	}
	if !strings.Contains(strings.ToLower(err.Error()), "unique") &&
		!strings.Contains(strings.ToLower(err.Error()), "duplicate key") {
		return err
	}
	existing, ok, getErr := s.getInboxDeliveryRaw(d.ChannelID, d.IdempotencyKey)
	if getErr != nil {
		return getErr
	}
	if !ok || InboxDeliveryFresh(existing, time.Now()) {
		return ErrInboxDeliveryExists
	}
	// Overwrite expired row for the same (channel_id, idempotency_key).
	_, err = s.exec(
		`UPDATE inbox_deliveries
		 SET delivery_id = ?, run_id = ?, body_hash = ?, created_at = ?
		 WHERE channel_id = ? AND idempotency_key = ?`,
		d.DeliveryID, d.RunID, d.BodyHash, d.CreatedAt.Format(time.RFC3339Nano),
		d.ChannelID, d.IdempotencyKey,
	)
	return err
}

func (s *SQLStore) UpdateInboxDelivery(channelID, idempotencyKey, runID string) error {
	res, err := s.exec(
		`UPDATE inbox_deliveries SET run_id = ? WHERE channel_id = ? AND idempotency_key = ?`,
		runID, channelID, idempotencyKey,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("inbox delivery not found")
	}
	return nil
}

func (s *SQLStore) GetInboxThread(channelID, externalID string) (string, bool, error) {
	var conversationID string
	err := s.queryRow(
		`SELECT conversation_id FROM inbox_threads WHERE channel_id = ? AND external_id = ?`,
		channelID, externalID,
	).Scan(&conversationID)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return conversationID, true, nil
}

func (s *SQLStore) PutInboxThread(channelID, externalID, conversationID string) error {
	_, err := s.exec(
		`INSERT INTO inbox_threads (channel_id, external_id, conversation_id)
		 VALUES (?, ?, ?)
		 ON CONFLICT(channel_id, external_id) DO UPDATE SET conversation_id = excluded.conversation_id`,
		channelID, externalID, conversationID,
	)
	return err
}
