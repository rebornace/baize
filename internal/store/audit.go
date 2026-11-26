package store

import (
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AuditEventRetention caps how many audit rows are kept. Administrative actions
// are low volume, so a fixed cap is enough and keeps the table bounded without
// a background job.
const AuditEventRetention = 5000

// AuditEvent records one state-changing request that passed authorization.
// It intentionally stores no request body: several audited endpoints accept
// credentials or provider keys.
type AuditEvent struct {
	ID         string    `json:"id"`
	OperatorID string    `json:"operator_id"`
	Role       string    `json:"role"`
	Method     string    `json:"method"`
	Action     string    `json:"action"`
	Target     string    `json:"target"`
	Status     int       `json:"status"`
	RemoteAddr string    `json:"remote_addr,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
}

// AuditFilter narrows the audit listing. Empty fields match everything.
type AuditFilter struct {
	Action     string
	OperatorID string
	// Query matches operator id, action or target with a case-insensitive
	// substring.
	Query string
	From  time.Time
	To    time.Time
	Limit int
}

func normalizeAuditEvent(e *AuditEvent, now time.Time) {
	if e.ID == "" {
		e.ID = uuid.NewString()
	}
	e.Method = strings.ToUpper(strings.TrimSpace(e.Method))
	e.Action = strings.TrimSpace(e.Action)
	e.Target = strings.TrimSpace(e.Target)
	e.OperatorID = strings.TrimSpace(e.OperatorID)
	e.Role = strings.TrimSpace(e.Role)
	if e.CreatedAt.IsZero() {
		e.CreatedAt = now
	}
	e.CreatedAt = e.CreatedAt.UTC()
}

func auditEventMatches(e AuditEvent, f AuditFilter) bool {
	if f.Action != "" && e.Action != f.Action {
		return false
	}
	if f.OperatorID != "" && e.OperatorID != f.OperatorID {
		return false
	}
	if !f.From.IsZero() && e.CreatedAt.Before(f.From) {
		return false
	}
	if !f.To.IsZero() && e.CreatedAt.After(f.To) {
		return false
	}
	if q := strings.ToLower(strings.TrimSpace(f.Query)); q != "" {
		haystack := strings.ToLower(e.OperatorID + "\n" + e.Action + "\n" + e.Target)
		if !strings.Contains(haystack, q) {
			return false
		}
	}
	return true
}

func normalizeAuditLimit(limit int) int {
	if limit <= 0 {
		return 100
	}
	if limit > AuditEventRetention {
		return AuditEventRetention
	}
	return limit
}

// AppendAuditEvent stores one event in memory and evicts the oldest rows past
// the retention cap.
func (s *Memory) AppendAuditEvent(e AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalizeAuditEvent(&e, time.Now().UTC())
	s.auditEvents = append(s.auditEvents, e)
	if over := len(s.auditEvents) - AuditEventRetention; over > 0 {
		s.auditEvents = append([]AuditEvent(nil), s.auditEvents[over:]...)
	}
	return nil
}

// TrimAuditEvents keeps only the newest `keep` in-memory events.
func (s *Memory) TrimAuditEvents(keep int) error {
	if keep <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.auditEvents) <= keep {
		return nil
	}
	s.auditEvents = append([]AuditEvent(nil), s.auditEvents[len(s.auditEvents)-keep:]...)
	return nil
}

// ListAuditEvents returns newest first.
func (s *Memory) ListAuditEvents(f AuditFilter) ([]AuditEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	limit := normalizeAuditLimit(f.Limit)
	out := make([]AuditEvent, 0, limit)
	for i := len(s.auditEvents) - 1; i >= 0; i-- {
		e := s.auditEvents[i]
		if !auditEventMatches(e, f) {
			continue
		}
		out = append(out, e)
		if len(out) >= limit {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}
