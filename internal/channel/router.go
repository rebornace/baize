package channel

import (
	"context"
	"strings"
	"sync"
)

// Router is a Channel that fans out to registered child channels, selecting
// the outbound target by conversation meta.Source. It implements Channel so the
// engine/UI can hold it as a single Outbound. Inbound (Start/Stop) is fanned out
// to all children; SendText/SendMedia are routed via Deliver* helpers (which use
// Router.For), so their direct invocation is a no-op (no implicit source).
type Router struct {
	mu       sync.RWMutex
	bySource map[string]Channel
	order    []Channel
	runtimes map[string]*Runtime // source -> Runtime（用于 Extras 解析 context_token）
}

// NewRouter returns an empty channel router.
func NewRouter() *Router {
	return &Router{bySource: map[string]Channel{}, runtimes: map[string]*Runtime{}}
}

// Add registers a child. Channels implementing SourceSourced are keyed by
// Source(); others are retained only for Start/Stop fan-out (not routable).
func (r *Router) Add(c Channel) {
	if c == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.order = append(r.order, c)
	if ss, ok := c.(SourceSourced); ok {
		if src := ss.Source(); src != "" {
			r.bySource[src] = c
		}
	}
}

// For returns the channel responsible for a conversation source.
func (r *Router) For(source string) (Channel, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.bySource[source]
	return c, ok
}

// Name identifies the router.
func (r *Router) Name() string { return "router" }

// Start fans out to all children (in-process channels start their own loops).
func (r *Router) Start(ctx context.Context) error {
	r.mu.RLock()
	chans := append([]Channel(nil), r.order...)
	r.mu.RUnlock()
	for _, c := range chans {
		if err := c.Start(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Stop fans out to all children.
func (r *Router) Stop(ctx context.Context) error {
	r.mu.RLock()
	chans := append([]Channel(nil), r.order...)
	r.mu.RUnlock()
	var firstErr error
	for _, c := range chans {
		if err := c.Stop(ctx); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SendText/SendMedia on the Router itself are no-ops: outbound delivery always
// goes through Deliver* which selects the child by meta.Source. Kept to satisfy
// the Channel interface.
func (r *Router) SendText(ctx context.Context, peerID, text string, extras map[string]string) error {
	return nil
}

func (r *Router) SendMedia(ctx context.Context, peerID, filename, mime string, data []byte, extras map[string]string) error {
	return nil
}

// BindRuntime registers a channel Runtime keyed by its Source (default "weixin")
// so Extras can resolve per-conversation context tokens.
func (r *Router) BindRuntime(rt *Runtime) {
	if rt == nil {
		return
	}
	src := strings.TrimSpace(rt.Source)
	if src == "" {
		src = "weixin"
	}
	r.mu.Lock()
	if r.runtimes == nil {
		r.runtimes = map[string]*Runtime{}
	}
	r.runtimes[src] = rt
	r.mu.Unlock()
}

// Extras returns per-conversation outbound extras from the Runtime owning the
// conversation's source (used as engine.OutboundExtras).
func (r *Router) Extras(conversationID string) map[string]string {
	src := SourceFromConvID(conversationID)
	r.mu.RLock()
	rt := r.runtimes[src]
	r.mu.RUnlock()
	if rt == nil {
		return nil
	}
	return rt.OutboundExtras(conversationID)
}
