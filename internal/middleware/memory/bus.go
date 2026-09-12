package memory

import (
	"context"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/middleware"
)

// bus wraps the in-process eventbus.Hub. For the memory driver the bus IS the
// hub: SSE/webhook still subscribe to the Hub directly, while the EventBus
// interface is satisfied by an in-process nudge channel bridge. Cross-replica
// injection is the redis driver's job (task 8/9).
type bus struct {
	hub *eventbus.Hub
	ch  chan middleware.RunEventNudge
}

func newBus(hub *eventbus.Hub) *bus {
	return &bus{hub: hub, ch: make(chan middleware.RunEventNudge, 256)}
}

func (b *bus) PublishRunEvent(_ context.Context, runID string, seq int64) error {
	select {
	case b.ch <- middleware.RunEventNudge{RunID: runID, Seq: seq}:
	default:
	}
	return nil
}

func (b *bus) SubscribeRunEvents(_ context.Context) (<-chan middleware.RunEventNudge, error) {
	return b.ch, nil
}

func (b *bus) Hub() *eventbus.Hub { return b.hub }
func (b *bus) Close() error       { return nil }
