package otelx

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace/noop"
)

// Controller holds the active Exporter behind a replaceable pointer so Hub
// observers are registered once and always forward to the current exporter.
type Controller struct {
	exp atomic.Pointer[Exporter]

	attachOnce sync.Once
	mu         sync.Mutex // serializes Apply / Shutdown
}

// NewController returns an empty controller (OTel off until Apply).
func NewController() *Controller {
	return &Controller{}
}

func (c *Controller) current() *Exporter {
	if c == nil {
		return nil
	}
	return c.exp.Load()
}

// AttachHub registers Hub observers once; callbacks forward to the current exporter.
func (c *Controller) AttachHub(hub *eventbus.Hub) {
	if c == nil || hub == nil {
		return
	}
	c.attachOnce.Do(func() {
		hub.OnEvent(func(runID string, ev eventbus.IndexedEvent) {
			if e := c.current(); e != nil {
				e.onEvent(runID, ev)
			}
		})
		hub.OnEnd(func(runID string, status store.Status) {
			if e := c.current(); e != nil {
				e.onEnd(runID, status)
			}
		})
	})
}

// Probe starts then immediately shuts down an exporter for cfg to validate the
// endpoint without installing it. Empty endpoint succeeds (no-op).
func Probe(ctx context.Context, cfg Config) error {
	e, err := Start(ctx, cfg)
	if err != nil {
		return err
	}
	return e.Shutdown(ctx)
}

// Apply replaces the active exporter. On Start failure the previous exporter
// remains. Empty endpoint disables export (nil exporter).
func (c *Controller) Apply(ctx context.Context, cfg Config) error {
	if c == nil {
		return nil
	}
	next, err := Start(ctx, cfg)
	if err != nil {
		return err
	}
	c.mu.Lock()
	old := c.exp.Swap(next)
	if next == nil {
		// Clear global provider so spans after disable are no-ops.
		otel.SetTracerProvider(noop.NewTracerProvider())
	}
	c.mu.Unlock()
	if old != nil {
		_ = old.Shutdown(ctx)
	}
	return nil
}

// RecordExportCall forwards to the current exporter (safe when nil / off).
func (c *Controller) RecordExportCall(toolName, exportKeyID, identityID, contractHash string, catalogRevision int64, isError bool) {
	if e := c.current(); e != nil {
		e.RecordExportCall(toolName, exportKeyID, identityID, contractHash, catalogRevision, isError)
	}
}

// Shutdown stops the active exporter.
func (c *Controller) Shutdown(ctx context.Context) error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	old := c.exp.Swap(nil)
	c.mu.Unlock()
	return old.Shutdown(ctx)
}

// Enabled reports whether an exporter is currently installed.
func (c *Controller) Enabled() bool {
	return c.current() != nil
}
