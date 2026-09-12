package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
)

func normalizeWebhookOutboxEntry(e *WebhookOutboxEntry, now time.Time) {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	if e.MaxAttempts <= 0 {
		e.MaxAttempts = WebhookOutboxMaxAttempts
	}
	if e.DeliveryKey == "" {
		e.DeliveryKey = WebhookOutboxDeliveryKey(e.RunID, e.Kind, e.EventIndex)
	}
	if e.Status == "" {
		e.Status = WebhookOutboxPending
	}
	if e.NextRetryAt.IsZero() {
		e.NextRetryAt = now
	}
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.UpdatedAt = now
}

func webhookOutboxActive(status WebhookOutboxStatus) bool {
	return status == WebhookOutboxPending || status == WebhookOutboxDelivered
}

func (s *Memory) PutWebhookOutboxIfAbsent(entry WebhookOutboxEntry) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.webhookOutbox == nil {
		s.webhookOutbox = map[string]WebhookOutboxEntry{}
	}
	now := time.Now().UTC()
	normalizeWebhookOutboxEntry(&entry, now)
	for _, existing := range s.webhookOutbox {
		if existing.DeliveryKey == entry.DeliveryKey && webhookOutboxActive(existing.Status) {
			return false, existing.ID, nil
		}
	}
	s.webhookOutbox[entry.ID] = entry
	return true, entry.ID, nil
}

func (s *Memory) ListWebhookOutboxDue(now time.Time, limit int) ([]WebhookOutboxEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	var due []WebhookOutboxEntry
	for _, e := range s.webhookOutbox {
		if e.Status != WebhookOutboxPending {
			continue
		}
		if e.NextRetryAt.After(now) {
			continue
		}
		due = append(due, e)
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].NextRetryAt.Equal(due[j].NextRetryAt) {
			return due[i].CreatedAt.Before(due[j].CreatedAt)
		}
		return due[i].NextRetryAt.Before(due[j].NextRetryAt)
	})
	if len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

func (s *Memory) ListWebhookOutbox(statuses []WebhookOutboxStatus, limit int) ([]WebhookOutboxEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	allowed := map[WebhookOutboxStatus]bool{}
	for _, st := range statuses {
		allowed[st] = true
	}
	var out []WebhookOutboxEntry
	for _, e := range s.webhookOutbox {
		if len(allowed) > 0 && !allowed[e.Status] {
			continue
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Memory) GetWebhookOutbox(id string) (WebhookOutboxEntry, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.webhookOutbox[id]
	if !ok {
		return WebhookOutboxEntry{}, ErrWebhookOutboxNotFound
	}
	return e, nil
}

func (s *Memory) UpdateWebhookOutbox(entry WebhookOutboxEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.webhookOutbox[entry.ID]; !ok {
		return ErrWebhookOutboxNotFound
	}
	entry.UpdatedAt = time.Now().UTC()
	s.webhookOutbox[entry.ID] = entry
	return nil
}

func (s *Memory) ResetWebhookOutboxRetry(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.webhookOutbox[id]
	if !ok {
		return ErrWebhookOutboxNotFound
	}
	now := time.Now().UTC()
	e.Status = WebhookOutboxPending
	e.Attempt = 0
	e.LastError = ""
	e.NextRetryAt = now
	e.UpdatedAt = now
	s.webhookOutbox[id] = e
	return nil
}

func (s *SQLStore) PutWebhookOutboxIfAbsent(entry WebhookOutboxEntry) (bool, string, error) {
	now := time.Now().UTC()
	normalizeWebhookOutboxEntry(&entry, now)

	var existingID, existingStatus sql.NullString
	err := s.queryRow(
		`SELECT id, status FROM webhook_outbox WHERE delivery_key = ?`, entry.DeliveryKey,
	).Scan(&existingID, &existingStatus)
	if err == nil {
		if webhookOutboxActive(WebhookOutboxStatus(existingStatus.String)) {
			return false, existingID.String, nil
		}
	} else if err != sql.ErrNoRows {
		return false, "", err
	}

	_, err = s.exec(`INSERT INTO webhook_outbox (
		id, delivery_key, run_id, kind, event_index, payload_json, target_url, headers_json,
		attempt, max_attempts, status, last_error, next_retry_at, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ID, entry.DeliveryKey, entry.RunID, string(entry.Kind), entry.EventIndex,
		string(entry.PayloadJSON), entry.TargetURL, string(entry.HeadersJSON),
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

func scanWebhookOutboxRow(sc interface {
	Scan(dest ...any) error
}) (WebhookOutboxEntry, error) {
	var e WebhookOutboxEntry
	var kind, status, payload, headers, nextRetry, created, updated string
	if err := sc.Scan(
		&e.ID, &e.DeliveryKey, &e.RunID, &kind, &e.EventIndex, &payload, &e.TargetURL, &headers,
		&e.Attempt, &e.MaxAttempts, &status, &e.LastError, &nextRetry, &created, &updated,
	); err != nil {
		return WebhookOutboxEntry{}, err
	}
	e.Kind = WebhookOutboxKind(kind)
	e.Status = WebhookOutboxStatus(status)
	e.PayloadJSON = []byte(payload)
	e.HeadersJSON = []byte(headers)
	var err error
	e.NextRetryAt, err = time.Parse(time.RFC3339Nano, nextRetry)
	if err != nil {
		return WebhookOutboxEntry{}, fmt.Errorf("parse next_retry_at: %w", err)
	}
	e.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return WebhookOutboxEntry{}, fmt.Errorf("parse created_at: %w", err)
	}
	e.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated)
	if err != nil {
		return WebhookOutboxEntry{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return e, nil
}

func (s *SQLStore) ListWebhookOutboxDue(now time.Time, limit int) ([]WebhookOutboxEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.query(`
		SELECT id, delivery_key, run_id, kind, event_index, payload_json, target_url, headers_json,
		       attempt, max_attempts, status, last_error, next_retry_at, created_at, updated_at
		FROM webhook_outbox
		WHERE status = ? AND next_retry_at <= ?
		ORDER BY next_retry_at ASC, created_at ASC
		LIMIT ?`, string(WebhookOutboxPending), now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebhookOutboxEntry
	for rows.Next() {
		e, err := scanWebhookOutboxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func joinPlaceholders(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

func (s *SQLStore) ListWebhookOutbox(statuses []WebhookOutboxStatus, limit int) ([]WebhookOutboxEntry, error) {
	if limit <= 0 {
		limit = 50
	}
	if len(statuses) == 0 {
		statuses = []WebhookOutboxStatus{WebhookOutboxDead, WebhookOutboxPending}
	}
	args := make([]any, 0, len(statuses)+1)
	placeholders := make([]string, len(statuses))
	for i, st := range statuses {
		placeholders[i] = "?"
		args = append(args, string(st))
	}
	args = append(args, limit)
	q := fmt.Sprintf(`
		SELECT id, delivery_key, run_id, kind, event_index, payload_json, target_url, headers_json,
		       attempt, max_attempts, status, last_error, next_retry_at, created_at, updated_at
		FROM webhook_outbox
		WHERE status IN (%s)
		ORDER BY updated_at DESC
		LIMIT ?`, joinPlaceholders(placeholders))
	rows, err := s.query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebhookOutboxEntry
	for rows.Next() {
		e, err := scanWebhookOutboxRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetWebhookOutbox(id string) (WebhookOutboxEntry, error) {
	row := s.queryRow(`
		SELECT id, delivery_key, run_id, kind, event_index, payload_json, target_url, headers_json,
		       attempt, max_attempts, status, last_error, next_retry_at, created_at, updated_at
		FROM webhook_outbox WHERE id = ?`, id)
	e, err := scanWebhookOutboxRow(row)
	if err == sql.ErrNoRows {
		return WebhookOutboxEntry{}, ErrWebhookOutboxNotFound
	}
	return e, err
}

func (s *SQLStore) UpdateWebhookOutbox(entry WebhookOutboxEntry) error {
	entry.UpdatedAt = time.Now().UTC()
	res, err := s.exec(`UPDATE webhook_outbox SET
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
		return ErrWebhookOutboxNotFound
	}
	return nil
}

func (s *SQLStore) ResetWebhookOutboxRetry(id string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.exec(`UPDATE webhook_outbox SET
		status = ?, attempt = 0, last_error = '', next_retry_at = ?, updated_at = ?
		WHERE id = ?`,
		string(WebhookOutboxPending), now, now, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrWebhookOutboxNotFound
	}
	return nil
}

// DecodeWebhookOutboxHeaders parses stored headers for delivery replay.
func DecodeWebhookOutboxHeaders(raw []byte) (map[string]string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	var out map[string]string
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
