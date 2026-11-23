// Package metricsx exposes low-cardinality Prometheus metrics for Runs and tools.
package metricsx

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
)

var durationBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10, 30, 60}

// Collector holds a private Registry (do not use prometheus.DefaultRegisterer).
type Collector struct {
	reg *prometheus.Registry

	RunsTotal            *prometheus.CounterVec
	RunDuration          prometheus.Histogram
	ToolInvocations      *prometheus.CounterVec
	ToolDuration         *prometheus.HistogramVec
	MCPExportInvocations *prometheus.CounterVec

	mu         sync.Mutex
	runStarts  map[string]time.Time
	toolStarts map[string]time.Time // key: runID+"\x00"+tool_call_id
}

// New constructs a Collector with an isolated registry and registered metrics.
func New() *Collector {
	reg := prometheus.NewRegistry()
	runsTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "baize_runs_total",
		Help: "Terminal run count by status.",
	}, []string{"status"})
	runDuration := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "baize_run_duration_seconds",
		Help:    "Run wall time from run.started to terminal status.",
		Buckets: durationBuckets,
	})
	toolInvocations := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "baize_tool_invocations_total",
		Help: "Tool invocations from chat tool.result and MCP export.",
	}, []string{"connector_id", "result"})
	toolDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "baize_tool_duration_seconds",
		Help:    "Tool execution time when paired with llm.tool_call.",
		Buckets: durationBuckets,
	}, []string{"connector_id"})
	mcpExport := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "baize_mcp_export_invocations_total",
		Help: "MCP export path tool invocations.",
	}, []string{"result"})

	reg.MustRegister(runsTotal, runDuration, toolInvocations, toolDuration, mcpExport)

	return &Collector{
		reg:                  reg,
		RunsTotal:            runsTotal,
		RunDuration:          runDuration,
		ToolInvocations:      toolInvocations,
		ToolDuration:         toolDuration,
		MCPExportInvocations: mcpExport,
		runStarts:            make(map[string]time.Time),
		toolStarts:           make(map[string]time.Time),
	}
}

// Reg returns the private registry (for tests that inspect label sets).
func (c *Collector) Reg() *prometheus.Registry {
	if c == nil {
		return nil
	}
	return c.reg
}

// Handler serves metrics from the private registry.
func (c *Collector) Handler() http.Handler {
	if c == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		})
	}
	return promhttp.HandlerFor(c.reg, promhttp.HandlerOpts{})
}

// AttachHub subscribes to run events and terminal status (same pattern as otelx).
func (c *Collector) AttachHub(hub *eventbus.Hub) {
	if c == nil || hub == nil {
		return
	}
	hub.OnEvent(func(runID string, ev eventbus.IndexedEvent) {
		c.onEvent(runID, ev)
	})
	hub.OnEnd(func(runID string, status store.Status) {
		c.onEnd(runID, status)
	})
}

// RecordExportCall records MCP export invocations (no chat run).
func (c *Collector) RecordExportCall(connectorID string, isError bool) {
	if c == nil {
		return
	}
	result := resultLabel(isError)
	cid := connectorLabel(connectorID)
	c.MCPExportInvocations.WithLabelValues(result).Inc()
	c.ToolInvocations.WithLabelValues(cid, result).Inc()
}

// ObserveToolResult increments tool counters (and duration when paired or explicit).
// durationSeconds is ignored when a paired llm.tool_call start exists; when runID
// is empty and no pairing applies, only the counter is updated.
func (c *Collector) ObserveToolResult(data map[string]any, durationSeconds float64) {
	if c == nil {
		return
	}
	c.observeToolResult("", data, durationSeconds)
}

func (c *Collector) onEvent(runID string, ev eventbus.IndexedEvent) {
	switch ev.Event.Type {
	case "run.started":
		c.mu.Lock()
		c.runStarts[runID] = time.Now()
		c.mu.Unlock()
	case "llm.tool_call":
		if tcID := toolCallIDFromCall(ev.Event.Data); tcID != "" {
			c.mu.Lock()
			c.toolStarts[toolStartKey(runID, tcID)] = time.Now()
			c.mu.Unlock()
		}
	case "tool.result":
		c.observeToolResult(runID, ev.Event.Data, 0)
	}
}

func (c *Collector) onEnd(runID string, status store.Status) {
	c.RunsTotal.WithLabelValues(string(status)).Inc()

	c.mu.Lock()
	if start, ok := c.runStarts[runID]; ok {
		c.RunDuration.Observe(time.Since(start).Seconds())
		delete(c.runStarts, runID)
	}
	prefix := runID + "\x00"
	for k := range c.toolStarts {
		if strings.HasPrefix(k, prefix) {
			delete(c.toolStarts, k)
		}
	}
	c.mu.Unlock()
}

func (c *Collector) observeToolResult(runID string, data map[string]any, durationSeconds float64) {
	isError, _ := data["is_error"].(bool)
	cid := connectorLabel(stringField(data, "connector_id"))
	result := resultLabel(isError)
	c.ToolInvocations.WithLabelValues(cid, result).Inc()

	var dur float64
	var haveDur bool
	if runID != "" {
		if tcID := stringField(data, "tool_call_id"); tcID != "" {
			key := toolStartKey(runID, tcID)
			c.mu.Lock()
			if start, ok := c.toolStarts[key]; ok {
				dur = time.Since(start).Seconds()
				haveDur = true
				delete(c.toolStarts, key)
			}
			c.mu.Unlock()
		}
	}
	if !haveDur && durationSeconds > 0 {
		dur = durationSeconds
		haveDur = true
	}
	if haveDur {
		c.ToolDuration.WithLabelValues(cid).Observe(dur)
	}
}

func toolStartKey(runID, toolCallID string) string {
	return runID + "\x00" + toolCallID
}

func toolCallIDFromCall(data map[string]any) string {
	if id := stringField(data, "tool_call_id"); id != "" {
		return id
	}
	return stringField(data, "id")
}

func stringField(data map[string]any, key string) string {
	v, _ := data[key].(string)
	return v
}

func connectorLabel(id string) string {
	if strings.TrimSpace(id) == "" {
		return "unknown"
	}
	return id
}

func resultLabel(isError bool) string {
	if isError {
		return "error"
	}
	return "ok"
}
