package identity

import "context"

type ctxKey int

const (
	ctxKeyConversationID ctxKey = iota
	ctxKeyForceIdentityID
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
