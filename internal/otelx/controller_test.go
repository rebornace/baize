package otelx

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestControllerSingleAttachForwardsAndSwap(t *testing.T) {
	hub := eventbus.NewHub()
	ctl := NewController()
	ctl.AttachHub(hub)
	ctl.AttachHub(hub) // must not double-register

	sr1 := tracetest.NewSpanRecorder()
	tp1 := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr1))
	t.Cleanup(func() { _ = tp1.Shutdown(context.Background()) })
	ctl.exp.Store(StartWithProvider(tp1))

	hub.Publish("r1", eventbus.IndexedEvent{Index: 1, Event: store.Event{Type: "run.started"}})
	hub.PublishEnd("r1", store.StatusSucceeded)
	if n := len(sr1.Ended()); n < 1 {
		t.Fatalf("spans after first exporter: %d", n)
	}

	sr2 := tracetest.NewSpanRecorder()
	tp2 := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr2))
	t.Cleanup(func() { _ = tp2.Shutdown(context.Background()) })
	if err := ctl.Apply(context.Background(), Config{}); err != nil {
		t.Fatal(err)
	}
	// Apply(empty) clears; reinstall second provider.
	ctl.exp.Store(StartWithProvider(tp2))

	hub.Publish("r2", eventbus.IndexedEvent{Index: 1, Event: store.Event{Type: "run.started"}})
	hub.PublishEnd("r2", store.StatusSucceeded)
	if n := len(sr2.Ended()); n < 1 {
		t.Fatalf("spans after second exporter: %d", n)
	}
	// First recorder should not grow from r2 (old exporter shut down / replaced).
	firstCount := len(sr1.Ended())
	hub.Publish("r3", eventbus.IndexedEvent{Index: 1, Event: store.Event{Type: "run.started"}})
	hub.PublishEnd("r3", store.StatusSucceeded)
	if len(sr1.Ended()) != firstCount {
		t.Fatalf("first exporter still receiving events")
	}
	if !ctl.Enabled() {
		t.Fatal("expected enabled after reinstall")
	}
	_ = ctl.Shutdown(context.Background())
}

func TestProbeEmptyOK(t *testing.T) {
	if err := Probe(context.Background(), Config{}); err != nil {
		t.Fatal(err)
	}
}

func TestApplyEmptyDisables(t *testing.T) {
	ctl := NewController()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })
	ctl.exp.Store(StartWithProvider(tp))
	if err := ctl.Apply(context.Background(), Config{}); err != nil {
		t.Fatal(err)
	}
	if ctl.Enabled() {
		t.Fatal("expected disabled")
	}
}
