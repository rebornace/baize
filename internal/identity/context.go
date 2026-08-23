package identity

import "context"

type ctxKey int

const (
	ctxKeyConversationID ctxKey = iota
	ctxKeyForceIdentityID
	ctxKeyRunID
	ctxKeyAgentID
	ctxKeyToolCallID
)

func WithConversationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyConversationID, id)
}

func ConversationIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyConversationID).(string)
	return v
}

func WithForceIdentityID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyForceIdentityID, id)
}

func ForceIdentityIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyForceIdentityID).(string)
	return v
}

func WithRunID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyRunID, id)
}

func RunIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyRunID).(string)
	return v
}

func WithAgentID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyAgentID, id)
}

func AgentIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyAgentID).(string)
	return v
}

func WithToolCallID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, ctxKeyToolCallID, id)
}

func ToolCallIDFrom(ctx context.Context) string {
	v, _ := ctx.Value(ctxKeyToolCallID).(string)
	return v
}

type passthroughKey struct{}

// WithPassthroughHeaders attaches per-run passthrough auth headers to ctx so
// connector invokers can use them as DefaultHeaders in passthrough mode.
func WithPassthroughHeaders(ctx context.Context, h map[string]string) context.Context {
	return context.WithValue(ctx, passthroughKey{}, h)
}

// PassthroughHeadersFrom returns the passthrough headers previously attached
// via WithPassthroughHeaders, or nil.
func PassthroughHeadersFrom(ctx context.Context) map[string]string {
	v, _ := ctx.Value(passthroughKey{}).(map[string]string)
	return v
}
