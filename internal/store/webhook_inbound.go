package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// WebhookInboundStatus tracks one durable inbound job.
type WebhookInboundStatus string

const (
	WebhookInboundPending WebhookInboundStatus = "pending"
	WebhookInboundDone    WebhookInboundStatus = "done"
	WebhookInboundDead    WebhookInboundStatus = "dead"
)

// WebhookInboundJob is a verified webhook request waiting to be processed
// (attachments resolved, run created). Body is the raw request JSON.
type WebhookInboundJob struct {
	ID            string
	DedupeKey     string
	Channel       string
	Body          []byte
	Status        WebhookInboundStatus
	Attempt       int
	MaxAttempts   int
	LastError     string
	NextAttemptAt time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func normalizeWebhookInboundJob(job *WebhookInboundJob, now time.Time) {
	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	if job.DedupeKey == "" {
		job.DedupeKey = "auto:" + uuid.NewString()
	}
	if job.Status == "" {
		job.Status = WebhookInboundPending
	}
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = 5
	}
	if job.NextAttemptAt.IsZero() {
		job.NextAttemptAt = now
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = now
	}
	job.UpdatedAt = now
}

func (s *Memory) PutWebhookInboundJobIfAbsent(job WebhookInboundJob) (bool, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.webhookInbound == nil {
		s.webhookInbound = map[string]WebhookInboundJob{}
	}
	normalizeWebhookInboundJob(&job, time.Now().UTC())
	for _, existing := range s.webhookInbound {
		if existing.DedupeKey == job.DedupeKey {
			return false, existing.ID, nil
		}
	}
	s.webhookInbound[job.ID] = job
	return true, job.ID, nil
}

func (s *Memory) ListWebhookInboundJobsDue(now time.Time, limit int) ([]WebhookInboundJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 20
	}
	var due []WebhookInboundJob
	for _, job := range s.webhookInbound {
		if job.Status != WebhookInboundPending || job.NextAttemptAt.After(now) {
			continue
		}
		due = append(due, job)
	}
	sort.Slice(due, func(i, j int) bool {
		if due[i].NextAttemptAt.Equal(due[j].NextAttemptAt) {
			return due[i].CreatedAt.Before(due[j].CreatedAt)
		}
		return due[i].NextAttemptAt.Before(due[j].NextAttemptAt)
	})
	if len(due) > limit {
		due = due[:limit]
	}
	return due, nil
}

func (s *Memory) ListWebhookInboundJobs(channel string, statuses []WebhookInboundStatus, limit int) ([]WebhookInboundJob, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	allowed := map[WebhookInboundStatus]bool{}
	for _, st := range statuses {
		allowed[st] = true
	}
	var out []WebhookInboundJob
	for _, job := range s.webhookInbound {
		if channel != "" && job.Channel != channel {
			continue
		}
		if len(allowed) > 0 && !allowed[job.Status] {
			continue
		}
		out = append(out, job)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].CreatedAt.After(out[j].CreatedAt)
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Memory) GetWebhookInboundJob(id string) (WebhookInboundJob, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.webhookInbound[id]
	return job, ok, nil
}

func (s *Memory) ResetWebhookInboundJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.webhookInbound[id]
	if !ok {
		return fmt.Errorf("webhook inbound job %s not found", id)
	}
	now := time.Now().UTC()
	job.Status = WebhookInboundPending
	job.Attempt = 0
	job.LastError = ""
	job.NextAttemptAt = now
	job.UpdatedAt = now
	s.webhookInbound[id] = job
	return nil
}

func (s *Memory) UpdateWebhookInboundJob(job WebhookInboundJob) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.webhookInbound == nil {
		return fmt.Errorf("webhook inbound job %s not found", job.ID)
	}
	if _, ok := s.webhookInbound[job.ID]; !ok {
		return fmt.Errorf("webhook inbound job %s not found", job.ID)
	}
	job.UpdatedAt = time.Now().UTC()
	s.webhookInbound[job.ID] = job
	return nil
}

func (s *Memory) DeleteWebhookInboundJobsBefore(cutoff time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, job := range s.webhookInbound {
		if job.Status == WebhookInboundPending {
			continue
		}
		if job.UpdatedAt.Before(cutoff) {
			delete(s.webhookInbound, id)
		}
	}
	return nil
}

const webhookInboundColumns = `id, dedupe_key, channel, body, status, attempt, max_attempts,
	last_error, next_attempt_at, created_at, updated_at`

func scanWebhookInboundJob(sc interface{ Scan(dest ...any) error }) (WebhookInboundJob, error) {
	var job WebhookInboundJob
	var status, lastError, next, created, updated string
	if err := sc.Scan(
		&job.ID, &job.DedupeKey, &job.Channel, &job.Body, &status, &job.Attempt,
		&job.MaxAttempts, &lastError, &next, &created, &updated,
	); err != nil {
		return WebhookInboundJob{}, err
	}
	job.Status = WebhookInboundStatus(status)
	job.LastError = lastError
	var err error
	if job.NextAttemptAt, err = time.Parse(time.RFC3339Nano, next); err != nil {
		return WebhookInboundJob{}, fmt.Errorf("parse next_attempt_at: %w", err)
	}
	if job.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return WebhookInboundJob{}, fmt.Errorf("parse created_at: %w", err)
	}
	if job.UpdatedAt, err = time.Parse(time.RFC3339Nano, updated); err != nil {
		return WebhookInboundJob{}, fmt.Errorf("parse updated_at: %w", err)
	}
	return job, nil
}

func (s *SQLStore) PutWebhookInboundJobIfAbsent(job WebhookInboundJob) (bool, string, error) {
	normalizeWebhookInboundJob(&job, time.Now().UTC())
	res, err := s.exec(`INSERT INTO webhook_inbound_jobs (`+webhookInboundColumns+`)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (dedupe_key) DO NOTHING`,
		job.ID, job.DedupeKey, job.Channel, string(job.Body), string(job.Status),
		job.Attempt, job.MaxAttempts, job.LastError,
		job.NextAttemptAt.UTC().Format(time.RFC3339Nano),
		job.CreatedAt.UTC().Format(time.RFC3339Nano),
		job.UpdatedAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return false, "", err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, "", err
	}
	if n > 0 {
		return true, job.ID, nil
	}
	var existing string
	if err := s.queryRow(
		`SELECT id FROM webhook_inbound_jobs WHERE dedupe_key = ?`, job.DedupeKey,
	).Scan(&existing); err != nil {
		return false, "", err
	}
	return false, existing, nil
}

func (s *SQLStore) ListWebhookInboundJobsDue(now time.Time, limit int) ([]WebhookInboundJob, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := s.query(`SELECT `+webhookInboundColumns+`
		FROM webhook_inbound_jobs
		WHERE status = ? AND next_attempt_at <= ?
		ORDER BY next_attempt_at ASC, created_at ASC
		LIMIT ?`,
		string(WebhookInboundPending), now.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebhookInboundJob
	for rows.Next() {
		job, err := scanWebhookInboundJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *SQLStore) ListWebhookInboundJobs(channel string, statuses []WebhookInboundStatus, limit int) ([]WebhookInboundJob, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `SELECT ` + webhookInboundColumns + ` FROM webhook_inbound_jobs`
	args := []any{}
	var conds []string
	if channel != "" {
		conds = append(conds, "channel = ?")
		args = append(args, channel)
	}
	if len(statuses) > 0 {
		ph := make([]string, len(statuses))
		for i, st := range statuses {
			ph[i] = "?"
			args = append(args, string(st))
		}
		conds = append(conds, "status IN ("+strings.Join(ph, ",")+")")
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += ` ORDER BY updated_at DESC, created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := s.query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WebhookInboundJob
	for rows.Next() {
		job, err := scanWebhookInboundJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, job)
	}
	return out, rows.Err()
}

func (s *SQLStore) GetWebhookInboundJob(id string) (WebhookInboundJob, bool, error) {
	job, err := scanWebhookInboundJob(s.queryRow(
		`SELECT `+webhookInboundColumns+` FROM webhook_inbound_jobs WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return WebhookInboundJob{}, false, nil
	}
	if err != nil {
		return WebhookInboundJob{}, false, err
	}
	return job, true, nil
}

func (s *SQLStore) ResetWebhookInboundJob(id string) error {
	now := time.Now().UTC()
	res, err := s.exec(`UPDATE webhook_inbound_jobs SET
		status = ?, attempt = 0, last_error = '', next_attempt_at = ?, updated_at = ?
		WHERE id = ?`,
		string(WebhookInboundPending), now.Format(time.RFC3339Nano),
		now.Format(time.RFC3339Nano), id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("webhook inbound job %s not found", id)
	}
	return nil
}

func (s *SQLStore) UpdateWebhookInboundJob(job WebhookInboundJob) error {
	_, err := s.exec(`UPDATE webhook_inbound_jobs SET
		status = ?, attempt = ?, last_error = ?, next_attempt_at = ?, updated_at = ?
		WHERE id = ?`,
		string(job.Status), job.Attempt, job.LastError,
		job.NextAttemptAt.UTC().Format(time.RFC3339Nano),
		time.Now().UTC().Format(time.RFC3339Nano), job.ID,
	)
	return err
}

func (s *SQLStore) DeleteWebhookInboundJobsBefore(cutoff time.Time) error {
	_, err := s.exec(
		`DELETE FROM webhook_inbound_jobs WHERE status <> ? AND updated_at < ?`,
		string(WebhookInboundPending), cutoff.UTC().Format(time.RFC3339Nano),
	)
	return err
}
