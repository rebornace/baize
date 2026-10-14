package otelx_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/otelx"
	"github.com/rebornace/baize/internal/store"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestStartEmptyEndpointNoOp(t *testing.T) {
	exp, err := otelx.Start(context.Background(), otelx.Config{})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if exp != nil {
		t.Fatal("expected nil exporter when endpoint empty")
	}
	var nilExp *otelx.Exporter
	nilExp.AttachHub(eventbus.NewHub())
	nilExp.RecordExportCall("t", "k", "i", false)
	if err := nilExp.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestAttachHubRecordsRunAndToolSpans(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	exp := otelx.StartWithProvider(tp)
	hub := eventbus.NewHub()
	exp.AttachHub(hub)

	runID := "run_otel_1"
	hub.Publish(runID, eventbus.IndexedEvent{
		Index: 1,
		Event: store.Event{Type: "run.started"},
	})
	hub.Publish(runID, eventbus.IndexedEvent{
		Index: 2,
		Event: store.Event{
			Type: "tool.result",
			Data: map[string]any{
				"name":         "get_ticket",
				"connector_id": "c1",
				"is_error":     false,
			},
		},
	})
	hub.PublishEnd(runID, store.StatusSucceeded)

	spans := sr.Ended()
	if len(spans) < 2 {
		t.Fatalf("ended spans = %d, want >= 2", len(spans))
	}
	var sawRun, sawTool bool
	for _, sp := range spans {
		switch sp.Name() {
		case "baize.run":
			sawRun = true
		case "baize.tool get_ticket":
			sawTool = true
			attrs := attrMap(sp)
			if attrs["baize.connector_id"] != "c1" {
				t.Fatalf("connector_id attr = %q", attrs["baize.connector_id"])
			}
		}
	}
	if !sawRun || !sawTool {
		t.Fatalf("sawRun=%v sawTool=%v spans=%v", sawRun, sawTool, spanNames(spans))
	}
}

func TestRecordExportCall(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	exp := otelx.StartWithProvider(tp)
	exp.RecordExportCall("list_orders", "key_1", "mei_1", true)

	spans := sr.Ended()
	if len(spans) != 1 {
		t.Fatalf("spans = %d, want 1", len(spans))
	}
	if spans[0].Name() != "baize.mcp_export list_orders" {
		t.Fatalf("name = %q", spans[0].Name())
	}
	attrs := attrMap(spans[0])
	if attrs["baize.export_key_id"] != "key_1" || attrs["baize.identity_id"] != "mei_1" {
		t.Fatalf("attrs = %#v", attrs)
	}
	if spans[0].Status().Code.String() != "Error" {
		t.Fatalf("status = %v, want Error", spans[0].Status())
	}
}

func attrMap(sp sdktrace.ReadOnlySpan) map[string]string {
	out := map[string]string{}
	for _, a := range sp.Attributes() {
		out[string(a.Key)] = a.Value.AsString()
	}
	return out
}

func spanNames(spans []sdktrace.ReadOnlySpan) []string {
	out := make([]string, len(spans))
	for i, sp := range spans {
		out[i] = sp.Name()
	}
	return out
}
