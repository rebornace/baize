package run

import (
	"context"

	"github.com/rebornace/baize/internal/controlplane"
)

// Decision is a human approval or rejection for a waiting run.
// Role and OperatorID identify the approver so tools that check control-plane
// ACL (e.g. propose_skill) see the resume principal after ContinueFromHITL,
// which otherwise drops the HTTP request context.
type Decision struct {
	Approve    bool
	Comment    string
	Role       controlplane.Role
	OperatorID string
}

// applyDecisionPrincipal copies the approver identity from d onto ctx when set.
func applyDecisionPrincipal(ctx context.Context, d Decision) context.Context {
	if d.Role != controlplane.RoleNone {
		ctx = controlplane.WithRole(ctx, d.Role)
	}
	if d.OperatorID != "" {
		ctx = controlplane.WithOperatorID(ctx, d.OperatorID)
	}
	return ctx
}
