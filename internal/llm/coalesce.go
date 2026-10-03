package llm

import "time"

// Coalescer rate-limits cumulative think/content callbacks.
// Think/Content store the latest snapshot and emit only when min has elapsed
// since the last emit of that channel; Flush always emits the final snapshots.
type Coalescer struct {
	min            time.Duration
	Now            func() time.Time // optional; defaults to time.Now
	lastT, lastC   time.Time
	think, content string
	emitT, emitC   func(string)
}

// NewCoalescer returns a Coalescer that invokes onThink/onContent at most once
// per min interval (plus a final Flush).
func NewCoalescer(min time.Duration, onThink, onContent func(string)) *Coalescer {
	return &Coalescer{
		min:   min,
		Now:   time.Now,
		emitT: onThink,
		emitC: onContent,
	}
}

func (c *Coalescer) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Think records a cumulative thinking snapshot and emits if the think interval elapsed.
func (c *Coalescer) Think(s string) {
	c.think = s
	now := c.now()
	if now.Sub(c.lastT) >= c.min {
		if c.emitT != nil {
			c.emitT(s)
		}
		c.lastT = now
	}
}

// Content records a cumulative content snapshot and emits if the content interval elapsed.
func (c *Coalescer) Content(s string) {
	c.content = s
	now := c.now()
	if now.Sub(c.lastC) >= c.min {
		if c.emitC != nil {
			c.emitC(s)
		}
		c.lastC = now
	}
}

// Flush emits the latest think and content snapshots (if non-empty).
func (c *Coalescer) Flush() {
	if c.think != "" && c.emitT != nil {
		c.emitT(c.think)
	}
	if c.content != "" && c.emitC != nil {
		c.emitC(c.content)
	}
}
