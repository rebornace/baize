package otelx_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/otelx"
	"github.com/rebornace/baize/internal/store"
	"go.opentelemetry.io/otel/attribute"
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
	nilExp.RecordExportCall("t", "k", "i", "", 0, false)
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

func TestToolResultSpanContractAttrs(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	exp := otelx.StartWithProvider(tp)
	hub := eventbus.NewHub()
	exp.AttachHub(hub)

	runID := "r1"
	hub.Publish(runID, eventbus.IndexedEvent{
		Index: 1,
		Event: store.Event{Type: "run.started"},
	})
	hub.Publish(runID, eventbus.IndexedEvent{
		Index: 2,
		Event: store.Event{
			Type: "tool.result",
			Data: map[string]any{
				"name":             "get_order",
				"connector_id":     "c1",
				"contract_hash":    "abcd1234efgh5678",
				"catalog_revision": int64(4),
				"is_error":         false,
			},
		},
	})

	var toolSpan sdktrace.ReadOnlySpan
	for _, sp := range sr.Ended() {
		if sp.Name() == "baize.tool get_order" {
			toolSpan = sp
			break
		}
	}
	if toolSpan == nil {
		t.Fatalf("missing baize.tool get_order span; got %v", spanNames(sr.Ended()))
	}
	attrs := attrMap(toolSpan)
	if attrs["baize.contract_hash"] != "abcd1234efgh5678" {
		t.Fatalf("contract_hash = %q", attrs["baize.contract_hash"])
	}
	if attrs["baize.catalog_revision"] != "4" {
		t.Fatalf("catalog_revision = %q", attrs["baize.catalog_revision"])
	}
}

func TestToolResultSpanCatalogRevisionTypes(t *testing.T) {
	cases := []struct {
		name string
		rev  any
		want string
	}{
		{"float64", float64(5), "5"},
		{"int", int(6), "6"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sr := tracetest.NewSpanRecorder()
			tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
			t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

			exp := otelx.StartWithProvider(tp)
			hub := eventbus.NewHub()
			exp.AttachHub(hub)

			runID := "r_" + tc.name
			hub.Publish(runID, eventbus.IndexedEvent{
				Index: 1,
				Event: store.Event{Type: "run.started"},
			})
			hub.Publish(runID, eventbus.IndexedEvent{
				Index: 2,
				Event: store.Event{
					Type: "tool.result",
					Data: map[string]any{
						"name":             "t",
						"catalog_revision": tc.rev,
					},
				},
			})

			for _, sp := range sr.Ended() {
				if sp.Name() != "baize.tool t" {
					continue
				}
				attrs := attrMap(sp)
				if attrs["baize.catalog_revision"] != tc.want {
					t.Fatalf("catalog_revision = %q, want %q", attrs["baize.catalog_revision"], tc.want)
				}
				return
			}
			t.Fatal("tool span not found")
		})
	}
}

func TestRecordExportCall(t *testing.T) {
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() { _ = tp.Shutdown(context.Background()) })

	exp := otelx.StartWithProvider(tp)
	exp.RecordExportCall("list_orders", "key_1", "mei_1", "abcd1234efgh5678", 3, true)

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
		switch a.Value.Type() {
		case attribute.INT64:
			out[string(a.Key)] = strconv.FormatInt(a.Value.AsInt64(), 10)
		default:
			out[string(a.Key)] = a.Value.AsString()
		}
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
