package eventbus

import (
	"sync"

	"github.com/rebornace/baize/internal/store"
)

// IndexedEvent is a store event with its 0-based index in the run's event list.
type IndexedEvent struct {
	Index int
	Event store.Event
}

// Subscription receives indexed events and a terminal status for one run.
type Subscription struct {
	Events <-chan IndexedEvent
	Ended  <-chan store.Status

	cancel func()
}

// Cancel unsubscribes and closes the subscription channels.
func (s *Subscription) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

type subscriber struct {
	events chan IndexedEvent
	ended  chan store.Status
}

// PendingSubscription receives a nudge whenever a HITL payload is set or
// cleared. The channel is buffered(1), so bursts coalesce.
type PendingSubscription struct {
	C <-chan struct{}

	cancel func()
}

// Cancel unsubscribes the pending listener.
func (s *PendingSubscription) Cancel() {
	if s.cancel != nil {
		s.cancel()
	}
}

type pendingSubscriber struct {
	ch chan struct{}
}

// PendingNudgeRunID is the reserved run id used to carry a cross-replica
// pending-approval nudge through the middleware event bus.
const PendingNudgeRunID = "__baize_pending_approvals__"

// Hub fans out AppendEvent / terminal UpdateRun notifications per run ID and
// pending-approval nudges to SSE subscribers.
type Hub struct {
	mu   sync.Mutex
	subs map[string][]*subscriber
	pend []*pendingSubscriber

	eventObservers   []func(runID string, ev IndexedEvent)
	endObservers     []func(runID string, status store.Status)
	pendingObservers []func()
}

// NewHub creates an empty in-process event hub.
func NewHub() *Hub {
	return &Hub{subs: make(map[string][]*subscriber)}
}

// Subscribe registers a buffered listener for runID. Buffer size is at least 16.
func (h *Hub) Subscribe(runID string) *Subscription {
	sub := &subscriber{
		events: make(chan IndexedEvent, 16),
		ended:  make(chan store.Status, 1),
	}
	h.mu.Lock()
	h.subs[runID] = append(h.subs[runID], sub)
	h.mu.Unlock()

	var once sync.Once
	cancel := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			list := h.subs[runID]
			for i, s := range list {
				if s == sub {
					h.subs[runID] = append(list[:i], list[i+1:]...)
					break
				}
			}
			if len(h.subs[runID]) == 0 {
				delete(h.subs, runID)
			}
			// Do not close channels: Publish may still hold a snapshot of this
			// subscriber; dropping avoids send-on-closed panic.
		})
	}

	return &Subscription{
		Events: sub.events,
		Ended:  sub.ended,
		cancel: cancel,
	}
}

// OnEvent registers a global observer invoked after each Publish.
func (h *Hub) OnEvent(fn func(runID string, ev IndexedEvent)) {
	h.mu.Lock()
	h.eventObservers = append(h.eventObservers, fn)
	h.mu.Unlock()
}

// OnEnd registers a global observer invoked after each PublishEnd.
func (h *Hub) OnEnd(fn func(runID string, status store.Status)) {
	h.mu.Lock()
	h.endObservers = append(h.endObservers, fn)
	h.mu.Unlock()
}

// PublishExternal injects a cross-replica nudge: subscribers of runID are told
// "new events exist up to index seq". The SSE handler re-reads the store, so the
// Event payload need not carry content; observers (webhook) also fire.
func (h *Hub) PublishExternal(runID string, seq int64) {
	h.Publish(runID, IndexedEvent{Index: int(seq), Event: store.Event{Type: "external.nudge"}})
}

// Publish delivers an indexed event to all subscribers of runID (non-blocking per sub).
func (h *Hub) Publish(runID string, ev IndexedEvent) {
	h.mu.Lock()
	list := append([]*subscriber(nil), h.subs[runID]...)
	obs := append([]func(runID string, ev IndexedEvent){}, h.eventObservers...)
	h.mu.Unlock()
	for _, sub := range list {
		select {
		case sub.events <- ev:
		default:
			// Drop if subscriber is slow; SSE handler dedupes via ListEvents indices.
		}
	}
	for _, fn := range obs {
		fn(runID, ev)
	}
}

// SubscribePending registers a listener for HITL pending-set changes.
func (h *Hub) SubscribePending() *PendingSubscription {
	sub := &pendingSubscriber{ch: make(chan struct{}, 1)}
	h.mu.Lock()
	h.pend = append(h.pend, sub)
	h.mu.Unlock()

	var once sync.Once
	return &PendingSubscription{
		C: sub.ch,
		cancel: func() {
			once.Do(func() {
				h.mu.Lock()
				defer h.mu.Unlock()
				for i, s := range h.pend {
					if s == sub {
						h.pend = append(h.pend[:i], h.pend[i+1:]...)
						break
					}
				}
			})
		},
	}
}

// OnPendingChange registers a local observer invoked after PublishPending.
// It is not invoked by PublishPendingExternal, preventing cross-replica echo.
func (h *Hub) OnPendingChange(fn func()) {
	h.mu.Lock()
	h.pendingObservers = append(h.pendingObservers, fn)
	h.mu.Unlock()
}

// PublishPending nudges pending-approval SSE subscribers and local observers.
// Bursts coalesce in the per-subscriber buffer.
func (h *Hub) PublishPending() {
	h.publishPending(true)
}

// PublishPendingExternal nudges local subscribers for a change that happened
// on another replica. It does not re-publish through the middleware bus.
func (h *Hub) PublishPendingExternal() {
	h.publishPending(false)
}

func (h *Hub) publishPending(notifyObservers bool) {
	h.mu.Lock()
	list := append([]*pendingSubscriber(nil), h.pend...)
	var obs []func()
	if notifyObservers {
		obs = append(obs, h.pendingObservers...)
	}
	h.mu.Unlock()
	for _, sub := range list {
		select {
		case sub.ch <- struct{}{}:
		default:
		}
	}
	for _, fn := range obs {
		fn()
	}
}

// PublishEnd notifies subscribers that the run reached a terminal status.
func (h *Hub) PublishEnd(runID string, status store.Status) {
	h.mu.Lock()
	list := append([]*subscriber(nil), h.subs[runID]...)
	obs := append([]func(runID string, status store.Status){}, h.endObservers...)
	h.mu.Unlock()
	for _, sub := range list {
		select {
		case sub.ended <- status:
		default:
		}
	}
	for _, fn := range obs {
		fn(runID, status)
	}
}
