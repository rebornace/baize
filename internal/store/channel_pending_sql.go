package store

import (
	"encoding/json"
	"fmt"
	"time"
)

const channelPendingColumns = `conversation_id, batch_id, source, account, peer_id, body, attachments_json, truncated, updated_at`

// UpsertChannelPending stores or updates one busy-merge batch.
func (s *SQLStore) UpsertChannelPending(p ChannelPending) error {
	if p.ConversationID == "" || p.BatchID == "" {
		return fmt.Errorf("channel pending: conversation_id and batch_id are required")
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	truncated := 0
	if p.Truncated {
		truncated = 1
	}
	attachments := p.Attachments
	if attachments == nil {
		attachments = []PendingAttachment{}
	}
	attachmentsJSON, err := json.Marshal(attachments)
	if err != nil {
		return fmt.Errorf("channel pending: marshal attachments: %w", err)
	}
	_, err = s.exec(`INSERT INTO channel_pending (`+channelPendingColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (conversation_id, batch_id) DO UPDATE SET
			source = excluded.source,
			account = excluded.account,
			peer_id = excluded.peer_id,
			body = excluded.body,
			attachments_json = excluded.attachments_json,
			truncated = excluded.truncated,
			updated_at = excluded.updated_at`,
		p.ConversationID, p.BatchID, p.Source, p.Account, p.PeerID, p.Text, string(attachmentsJSON), truncated,
		p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	return err
}

// ListChannelPending returns batches for source (all sources when empty),
// oldest first.
func (s *SQLStore) ListChannelPending(source string) ([]ChannelPending, error) {
	query := `SELECT ` + channelPendingColumns + ` FROM channel_pending`
	args := []any{}
	if source != "" {
		query += ` WHERE source = ?`
		args = append(args, source)
	}
	query += ` ORDER BY updated_at ASC, conversation_id ASC, batch_id ASC`
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ChannelPending
	for rows.Next() {
		var p ChannelPending
		var truncated int
		var updated, attachmentsJSON string
		if err := rows.Scan(
			&p.ConversationID, &p.BatchID, &p.Source, &p.Account, &p.PeerID,
			&p.Text, &attachmentsJSON, &truncated, &updated,
		); err != nil {
			return nil, err
		}
		if attachmentsJSON != "" {
			if err := json.Unmarshal([]byte(attachmentsJSON), &p.Attachments); err != nil {
				return nil, fmt.Errorf("parse channel_pending.attachments_json: %w", err)
			}
		}
		p.Truncated = truncated != 0
		p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
		if err != nil {
			return nil, fmt.Errorf("parse channel_pending.updated_at: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// DeleteChannelPending removes one batch (idempotent).
func (s *SQLStore) DeleteChannelPending(conversationID, batchID string) error {
	_, err := s.exec(
		`DELETE FROM channel_pending WHERE conversation_id = ? AND batch_id = ?`,
		conversationID, batchID,
	)
	return err
}
