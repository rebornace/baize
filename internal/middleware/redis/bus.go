package redis

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/middleware"
	goredis "github.com/redis/go-redis/v9"
)

// bus fans RunEventNudge over Redis Pub/Sub and optionally bridges into a
// local eventbus.Hub for SSE/webhook subscribers on this replica.
type bus struct {
	client  *goredis.Client
	channel string

	mu      sync.Mutex
	sub     *goredis.PubSub
	out     chan middleware.RunEventNudge
	hub     *eventbus.Hub
	started bool
}

func newBus(c *goredis.Client, ch string) *bus {
	return &bus{client: c, channel: ch, out: make(chan middleware.RunEventNudge, 256)}
}

func (b *bus) PublishRunEvent(ctx context.Context, runID string, seq int64) error {
	payload, err := json.Marshal(middleware.RunEventNudge{RunID: runID, Seq: seq})
	if err != nil {
		return err
	}
	return b.client.Publish(ctx, b.channel, payload).Err()
}

func (b *bus) SubscribeRunEvents(ctx context.Context) (<-chan middleware.RunEventNudge, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.started {
		return b.out, nil
	}
	sub := b.client.Subscribe(ctx, b.channel)
	b.sub = sub
	b.started = true
	go func() {
		ch := sub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				var n middleware.RunEventNudge
				if json.Unmarshal([]byte(msg.Payload), &n) != nil {
					continue
				}
				select {
				case b.out <- n:
				default:
				}
				b.mu.Lock()
				hub := b.hub
				b.mu.Unlock()
				if hub != nil {
					hub.PublishExternal(n.RunID, n.Seq)
				}
			}
		}
	}()
	return b.out, nil
}

// BridgeToHub wires cross-replica nudges into the local in-process Hub so SSE
// and webhook subscribers see remote events.
func (b *bus) BridgeToHub(hub *eventbus.Hub) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.hub = hub
}

// Start begins consuming the Pub/Sub channel. The returned nudge channel is not
// needed by bootstrap (nudges are injected into the Hub via BridgeToHub), so it
// is intentionally discarded. Must be called after BridgeToHub.
func (b *bus) Start(ctx context.Context) {
	_, _ = b.SubscribeRunEvents(ctx)
}

func (b *bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.sub != nil {
		err := b.sub.Close()
		b.sub = nil
		return err
	}
	return nil
}
