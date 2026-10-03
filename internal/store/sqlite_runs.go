package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// runSelectColumns is the canonical runs column order scanned by scanRunRow;
// GetRun and ListRunsForReconcile must select exactly these columns in order.
const runSelectColumns = `id, agent_id, input, status, output, error, created_at, conversation_id, identity_id, passthrough_json, webhook_json, model_profile_id, lease_until, thinking_level`

// scanRunRow decodes one runs row selected via runSelectColumns.
func scanRunRow(sc interface{ Scan(dest ...any) error }) (*Run, error) {
	var r Run
	var status, createdAt string
	var conversationID, identityID, passthroughSQL, webhookSQL, modelProfileID, thinkingLevel sql.NullString
	var leaseUntil sql.NullTime
	if err := sc.Scan(
		&r.ID, &r.AgentID, &r.Input, &status, &r.Output, &r.Error, &createdAt,
		&conversationID, &identityID, &passthroughSQL, &webhookSQL, &modelProfileID, &leaseUntil, &thinkingLevel,
	); err != nil {
		return nil, err
	}
	r.Status = Status(status)
	if conversationID.Valid {
		r.ConversationID = conversationID.String
	}
	if identityID.Valid {
		r.IdentityID = identityID.String
	}
	if modelProfileID.Valid {
		r.ModelProfileID = modelProfileID.String
	}
	if thinkingLevel.Valid {
		r.ThinkingLevel = thinkingLevel.String
	}
	if passthroughSQL.Valid && passthroughSQL.String != "" && passthroughSQL.String != "null" {
		if err := json.Unmarshal([]byte(passthroughSQL.String), &r.PassthroughHeaders); err != nil {
			return nil, fmt.Errorf("parse passthrough_json: %w", err)
		}
	}
	if webhookSQL.Valid && webhookSQL.String != "" && webhookSQL.String != "null" {
		var wc WebhookConfig
		if err := json.Unmarshal([]byte(webhookSQL.String), &wc); err != nil {
			return nil, fmt.Errorf("parse webhook_json: %w", err)
		}
		r.WebhookConfig = &wc
	}
	ts, err := time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		ts, err = time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse created_at: %w", err)
		}
	}
	r.CreatedAt = ts
	if leaseUntil.Valid {
		t := leaseUntil.Time.UTC()
		r.LeaseUntil = &t
	}
	return &r, nil
}

func scanRuns(rows *sql.Rows) ([]*Run, error) {
	defer rows.Close()
	var out []*Run
	for rows.Next() {
		r, err := scanRunRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLStore) CreateRun(in CreateRunInput) (*Run, error) {
	id := "run_" + uuid.NewString()
	now := time.Now().UTC()
	r := &Run{
		ID:                 id,
		AgentID:            in.AgentID,
		Input:              in.Input,
		Status:             StatusRunning,
		CreatedAt:          now,
		ConversationID:     in.ConversationID,
		IdentityID:         in.IdentityID,
		ModelProfileID:     in.ModelProfileID,
		ThinkingLevel:      in.ThinkingLevel,
		PassthroughHeaders: cloneHeaders(in.PassthroughHeaders),
		WebhookConfig:      cloneWebhookConfig(in.WebhookConfig),
	}
	var passthroughSQL, webhookSQL sql.NullString
	if len(r.PassthroughHeaders) > 0 {
		b, err := json.Marshal(r.PassthroughHeaders)
		if err != nil {
			return nil, err
		}
		passthroughSQL = sql.NullString{String: string(b), Valid: true}
	}
	if r.WebhookConfig != nil {
		b, err := json.Marshal(r.WebhookConfig)
		if err != nil {
			return nil, err
		}
		webhookSQL = sql.NullString{String: string(b), Valid: true}
	}
	_, err := s.exec(
		`INSERT INTO runs (id, agent_id, input, status, output, error, created_at, hitl_json, conversation_id, identity_id, passthrough_json, webhook_json, model_profile_id, thinking_level)
		 VALUES (?, ?, ?, ?, '', '', ?, NULL, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.AgentID, r.Input, string(r.Status), formatSQLiteTime(r.CreatedAt),
		r.ConversationID, r.IdentityID, passthroughSQL, webhookSQL, r.ModelProfileID, r.ThinkingLevel,
	)
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *SQLStore) GetRun(id string) (*Run, error) {
	r, err := scanRunRow(s.queryRow(
		`SELECT `+runSelectColumns+` FROM runs WHERE id = ?`, id,
	))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	return r, nil
}

func (s *SQLStore) SetPassthroughHeaders(id string, headers map[string]string) error {
	var passthroughSQL sql.NullString
	if len(headers) > 0 {
		b, err := json.Marshal(headers)
		if err != nil {
			return err
		}
		passthroughSQL = sql.NullString{String: string(b), Valid: true}
	}
	res, err := s.exec(
		`UPDATE runs SET passthrough_json = ? WHERE id = ?`,
		passthroughSQL, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("run not found")
	}
	return nil
}

func (s *SQLStore) UpdateRun(id string, status Status, output, errMsg string) error {
	res, err := s.exec(
		`UPDATE runs SET status = ?, output = ?, error = ? WHERE id = ?`,
		string(status), output, errMsg, id,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return fmt.Errorf("run not found")
	}
	return nil
}

func (s *SQLStore) LeaseRun(id string, ttl time.Duration) (bool, error) {
	now := time.Now().UTC()
	res, err := s.exec(
		`UPDATE runs SET lease_until = ?
		 WHERE id = ? AND status IN ('queued','running')
		   AND (lease_until IS NULL OR lease_until < ?)`,
		s.timeArg(now.Add(ttl)), id, s.timeArg(now),
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *SQLStore) HeartbeatRun(id string, ttl time.Duration) (bool, error) {
	res, err := s.exec(
		`UPDATE runs SET lease_until = ? WHERE id = ?`,
		s.timeArg(time.Now().UTC().Add(ttl)), id,
	)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *SQLStore) ClearRunLease(id string) error {
	_, err := s.exec(`UPDATE runs SET lease_until = NULL WHERE id = ?`, id)
	return err
}

func (s *SQLStore) ListRunsForReconcile(limit int) ([]*Run, error) {
	if limit <= 0 {
		limit = 100
	}
	now := time.Now().UTC()
	// Both columns are compared as TEXT on SQLite (fixed-width format above);
	// on postgres lease_until is native TIMESTAMPTZ while created_at stays TEXT,
	// so the grace cutoff is bound as an RFC3339 string either way.
	rows, err := s.query(
		`SELECT `+runSelectColumns+`
		 FROM runs
		 WHERE status IN ('queued','running')
		   AND (lease_until < ? OR (lease_until IS NULL AND created_at < ?))
		 ORDER BY created_at ASC LIMIT ?`,
		s.timeArg(now), formatSQLiteTime(now.Add(-reconcileGrace)), limit,
	)
	if err != nil {
		return nil, err
	}
	return scanRuns(rows)
}

func (s *SQLStore) AppendEvent(runID string, ev Event) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	ev.Timestamp = time.Now().UTC()
	dataJSON, err := json.Marshal(ev.Data)
	if err != nil {
		return err
	}
	_, err = s.exec(
		`INSERT INTO events (run_id, type, timestamp, data_json) VALUES (?, ?, ?, ?)`,
		runID, ev.Type, ev.Timestamp.Format(time.RFC3339Nano), string(dataJSON),
	)
	return err
}

func (s *SQLStore) ListEvents(runID string) ([]Event, error) {
	if _, err := s.GetRun(runID); err != nil {
		return nil, err
	}
	rows, err := s.query(
		`SELECT type, timestamp, data_json FROM events WHERE run_id = ? ORDER BY id ASC`,
		runID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Event
	for rows.Next() {
		var ev Event
		var ts, dataJSON string
		if err := rows.Scan(&ev.Type, &ts, &dataJSON); err != nil {
			return nil, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, ts)
			if err != nil {
				return nil, fmt.Errorf("parse event timestamp: %w", err)
			}
		}
		ev.Timestamp = parsed
		if dataJSON != "" && dataJSON != "null" {
			if err := json.Unmarshal([]byte(dataJSON), &ev.Data); err != nil {
				return nil, err
			}
		}
		out = append(out, ev)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []Event{}
	}
	return out, nil
}

func (s *SQLStore) SetHITL(runID string, payload *HITLPayload) error {
	if _, err := s.GetRun(runID); err != nil {
		return err
	}
	var hitlSQL sql.NullString
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		hitlSQL = sql.NullString{String: string(b), Valid: true}
	}
	_, err := s.exec(`UPDATE runs SET hitl_json = ? WHERE id = ?`, hitlSQL, runID)
	return err
}

func (s *SQLStore) GetHITL(runID string) (*HITLPayload, error) {
	var hitlSQL sql.NullString
	err := s.queryRow(`SELECT hitl_json FROM runs WHERE id = ?`, runID).Scan(&hitlSQL)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("run not found")
	}
	if err != nil {
		return nil, err
	}
	if !hitlSQL.Valid || hitlSQL.String == "" || hitlSQL.String == "null" {
		return nil, nil
	}
	var p HITLPayload
	if err := json.Unmarshal([]byte(hitlSQL.String), &p); err != nil {
		return nil, err
	}
	return &p, nil
}
