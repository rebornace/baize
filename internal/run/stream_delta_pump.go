package run

import (
	"sync"

	"github.com/rebornace/baize/internal/store"
)

// streamDeltaPump applies llm.*.delta upserts off the LLM SSE read loop.
// Callbacks only stash the latest snapshot and wake a single writer goroutine,
// so SQLite / hub fan-out cannot stall upstream token consumption.
type streamDeltaPump struct {
	store store.Store
	runID string
	turn  int

	mu       sync.Mutex
	think    string
	content  string
	thinkQ   bool
	contentQ bool
	closing  bool
	wake     chan struct{}
	done     chan struct{}
}

func newStreamDeltaPump(st store.Store, runID string, turn int) *streamDeltaPump {
	p := &streamDeltaPump{
		store: st,
		runID: runID,
		turn:  turn,
		wake:  make(chan struct{}, 1),
		done:  make(chan struct{}),
	}
	go p.loop()
	return p
}

func (p *streamDeltaPump) Think(s string) {
	p.mu.Lock()
	p.think = s
	p.thinkQ = true
	p.mu.Unlock()
	p.signal()
}

func (p *streamDeltaPump) Content(s string) {
	p.mu.Lock()
	p.content = s
	p.contentQ = true
	p.mu.Unlock()
	p.signal()
}

func (p *streamDeltaPump) signal() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

func (p *streamDeltaPump) loop() {
	defer close(p.done)
	for range p.wake {
		for {
			p.mu.Lock()
			think, tq := p.think, p.thinkQ
			content, cq := p.content, p.contentQ
			closing := p.closing
			p.thinkQ, p.contentQ = false, false
			p.mu.Unlock()

			if tq {
				_, _ = p.store.UpsertStreamingDelta(p.runID, store.Event{
					Type: EventLLMThinkingDelta,
					Data: map[string]any{"turn": p.turn, "text": think},
				})
			}
			if cq {
				_, _ = p.store.UpsertStreamingDelta(p.runID, store.Event{
					Type: EventLLMContentDelta,
					Data: map[string]any{"turn": p.turn, "text": content},
				})
			}
			if !tq && !cq {
				if closing {
					return
				}
				break
			}
		}
	}
}

// Close drains the latest snapshots and waits for the writer to finish.
func (p *streamDeltaPump) Close() {
	p.mu.Lock()
	p.closing = true
	p.mu.Unlock()
	p.signal()
	<-p.done
}
