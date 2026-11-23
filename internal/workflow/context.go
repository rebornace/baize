package workflow

import "context"

type stepStateKey struct{}

// WithStepState carries the current workflow step through tool invocation and
// HITL persistence. Any approval or login pause created while this context is
// active can persist the workflow position without every caller passing it
// explicitly.
func WithStepState(ctx context.Context, s State) context.Context {
	return context.WithValue(ctx, stepStateKey{}, s)
}

// StepStateFromContext returns the workflow step carried by ctx.
func StepStateFromContext(ctx context.Context) (State, bool) {
	s, ok := ctx.Value(stepStateKey{}).(State)
	return s, ok
}
