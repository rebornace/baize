package metricsx_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/metricsx"
	"github.com/rebornace/baize/internal/store"
)

func TestRunEndIncrementsCounter(t *testing.T) {
	m := metricsx.New()
	hub := eventbus.NewHub()
	m.AttachHub(hub)
	runID := "r1"
	hub.Publish(runID, eventbus.IndexedEvent{
		Index: 0,
		Event: store.Event{Type: "run.started"},
	})
	hub.PublishEnd(runID, store.StatusSucceeded)
	got := testutil.ToFloat64(m.RunsTotal.WithLabelValues("succeeded"))
	if got != 1 {
		t.Fatalf("runs_total=%v", got)
	}
}

func TestToolResultLabelsNoToolName(t *testing.T) {
	m := metricsx.New()
	m.ObserveToolResult(map[string]any{
		"connector_id": "c1",
		"is_error":     false,
		"name":         "create_ticket",
	}, 0)
	got := testutil.ToFloat64(m.ToolInvocations.WithLabelValues("c1", "ok"))
	if got != 1 {
		t.Fatalf("tool invocations=%v", got)
	}
	metric, err := m.Reg().Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range metric {
		if mf.GetName() != "baize_tool_invocations_total" {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "name" || lp.GetName() == "tool" {
					t.Fatalf("unexpected high-cardinality label %q", lp.GetName())
				}
			}
		}
	}
}

func TestEmptyConnectorIDIsUnknown(t *testing.T) {
	m := metricsx.New()
	m.ObserveToolResult(map[string]any{"is_error": true}, 0)
	got := testutil.ToFloat64(m.ToolInvocations.WithLabelValues("unknown", "error"))
	if got != 1 {
		t.Fatalf("tool invocations=%v", got)
	}
}

func TestToolDurationFromPairedToolCall(t *testing.T) {
	m := metricsx.New()
	hub := eventbus.NewHub()
	m.AttachHub(hub)
	runID := "run_tool_1"
	hub.Publish(runID, eventbus.IndexedEvent{
		Index: 1,
		Event: store.Event{
			Type: "llm.tool_call",
			Data: map[string]any{"id": "tc1", "name": "get_ticket"},
		},
	})
	hub.Publish(runID, eventbus.IndexedEvent{
		Index: 2,
		Event: store.Event{
			Type: "tool.result",
			Data: map[string]any{
				"tool_call_id": "tc1",
				"connector_id": "c1",
				"is_error":     false,
			},
		},
	})
	got := testutil.ToFloat64(m.ToolInvocations.WithLabelValues("c1", "ok"))
	if got != 1 {
		t.Fatalf("tool invocations=%v", got)
	}
	if !histogramSampleCount(t, m, "baize_tool_duration_seconds", "c1") {
		t.Fatal("expected tool duration histogram observation")
	}
}

func histogramSampleCount(t *testing.T, m *metricsx.Collector, name, connectorID string) bool {
	t.Helper()
	mfs, err := m.Reg().Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, met := range mf.GetMetric() {
			var cid string
			for _, lp := range met.GetLabel() {
				if lp.GetName() == "connector_id" {
					cid = lp.GetValue()
				}
			}
			if cid != connectorID {
				continue
			}
			if h := met.GetHistogram(); h != nil && h.GetSampleCount() > 0 {
				return true
			}
		}
	}
	return false
}

func TestRecordExportCall(t *testing.T) {
	m := metricsx.New()
	m.RecordExportCall("conn_a", false)
	if got := testutil.ToFloat64(m.MCPExportInvocations.WithLabelValues("ok")); got != 1 {
		t.Fatalf("mcp export=%v", got)
	}
	if got := testutil.ToFloat64(m.ToolInvocations.WithLabelValues("conn_a", "ok")); got != 1 {
		t.Fatalf("tool invocations=%v", got)
	}
}
