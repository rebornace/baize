package hostedlogin

import (
	"log"
	"strings"
)

// Audit event kinds for hosted login (no secrets in messages).
const (
	AuditSessionIssue = "hosted_login.session_issue"
	AuditSubmitOK     = "hosted_login.submit_ok"
	AuditSubmitFail   = "hosted_login.submit_fail"
	AuditRateLimited  = "hosted_login.rate_limited"
)

// AuditEvent is a structured hosted-login audit record without passwords/tokens.
type AuditEvent struct {
	Kind           string
	SessionID      string
	ConnectorID    string
	ConversationID string
	ExportKeyID    string
	ClientIP       string
	Err            string
}

// Auditor records audit events. nil is safe (no-op via audit helper).
type Auditor interface {
	Record(AuditEvent)
}

// LogAuditor writes audit lines to the standard logger.
type LogAuditor struct{}

// Record implements Auditor.
func (LogAuditor) Record(e AuditEvent) {
	parts := []string{"event=" + e.Kind}
	if e.SessionID != "" {
		parts = append(parts, "session="+e.SessionID)
	}
	if e.ConnectorID != "" {
		parts = append(parts, "connector="+e.ConnectorID)
	}
	if e.ConversationID != "" {
		parts = append(parts, "conversation="+e.ConversationID)
	}
	if e.ExportKeyID != "" {
		parts = append(parts, "export_key="+e.ExportKeyID)
	}
	if e.ClientIP != "" {
		parts = append(parts, "ip="+e.ClientIP)
	}
	if e.Err != "" {
		// Strip any accidental credential fragments from error text.
		msg := e.Err
		if strings.Contains(strings.ToLower(msg), "password") ||
			strings.Contains(strings.ToLower(msg), "token") ||
			strings.Contains(strings.ToLower(msg), "authorization") {
			msg = "redacted_error"
		}
		parts = append(parts, "err="+msg)
	}
	log.Printf("hostedlogin audit: %s", strings.Join(parts, " "))
}

func audit(a Auditor, e AuditEvent) {
	if a == nil {
		return
	}
	a.Record(e)
}
