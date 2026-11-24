package run

import (
	"context"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/llm"
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
	// UserParts optionally rehydrates the original multimodal user turn when a
	// run resumes (HITL) after the live job payload is gone. Nil is the default.
	UserParts []llm.ContentPart
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
