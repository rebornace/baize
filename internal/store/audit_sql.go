package store

import (
	"fmt"
	"strings"
	"time"
)

func (s *SQLStore) AppendAuditEvent(e AuditEvent) error {
	normalizeAuditEvent(&e, time.Now().UTC())
	if _, err := s.exec(`INSERT INTO audit_events (
		id, operator_id, role, method, action, target, status, remote_addr, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.OperatorID, e.Role, e.Method, e.Action, e.Target,
		e.Status, e.RemoteAddr, formatSQLiteTime(e.CreatedAt),
	); err != nil {
		return err
	}
	// Trim in the same call so the table stays bounded even when reads never
	// happen. Mutating admin requests are infrequent, so this is cheap.
	_, err := s.exec(fmt.Sprintf(`DELETE FROM audit_events WHERE id NOT IN (
		SELECT id FROM audit_events ORDER BY created_at DESC LIMIT %d
	)`, AuditEventRetention))
	return err
}

// TrimAuditEvents deletes rows outside the newest `keep`.
func (s *SQLStore) TrimAuditEvents(keep int) error {
	if keep <= 0 {
		return nil
	}
	_, err := s.exec(`DELETE FROM audit_events WHERE id NOT IN (
		SELECT id FROM audit_events ORDER BY created_at DESC LIMIT ?
	)`, keep)
	return err
}

func (s *SQLStore) ListAuditEvents(f AuditFilter) ([]AuditEvent, error) {
	limit := normalizeAuditLimit(f.Limit)
	q := `SELECT id, operator_id, role, method, action, target, status, remote_addr, created_at
		FROM audit_events WHERE 1 = 1`
	args := []any{}
	if f.Action != "" {
		q += ` AND action = ?`
		args = append(args, f.Action)
	}
	if f.OperatorID != "" {
		q += ` AND operator_id = ?`
		args = append(args, f.OperatorID)
	}
	if query := strings.TrimSpace(f.Query); query != "" {
		like := "%" + strings.ToLower(query) + "%"
		q += ` AND (LOWER(operator_id) LIKE ? OR LOWER(action) LIKE ? OR LOWER(target) LIKE ?)`
		args = append(args, like, like, like)
	}
	if !f.From.IsZero() {
		q += ` AND created_at >= ?`
		args = append(args, formatSQLiteTime(f.From))
	}
	if !f.To.IsZero() {
		q += ` AND created_at <= ?`
		args = append(args, formatSQLiteTime(f.To))
	}
	q += fmt.Sprintf(` ORDER BY created_at DESC LIMIT %d`, limit)
	rows, err := s.query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		var created string
		if err := rows.Scan(
			&e.ID, &e.OperatorID, &e.Role, &e.Method, &e.Action,
			&e.Target, &e.Status, &e.RemoteAddr, &created,
		); err != nil {
			return nil, err
		}
		e.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
		if err != nil {
			return nil, fmt.Errorf("parse audit created_at: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
