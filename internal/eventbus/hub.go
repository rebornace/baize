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

// Hub fans out AppendEvent / terminal UpdateRun notifications per run ID.
type Hub struct {
	mu   sync.Mutex
	subs map[string][]*subscriber

	eventObservers []func(runID string, ev IndexedEvent)
	endObservers   []func(runID string, status store.Status)
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
