// Package otelx optionally exports Run/tool events as OpenTelemetry spans.
// When Endpoint is empty, Start is a no-op and Record helpers do nothing.
package otelx

import (
	"context"
	"log/slog"
	"strings"
	"sync"

	"github.com/rebornace/baize/internal/applog"
	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Config selects the OTLP HTTP exporter. Empty Endpoint disables export.
type Config struct {
	Endpoint    string
	ServiceName string
}

// Exporter fans run/tool events into spans. Safe for concurrent use after Start.
type Exporter struct {
	tp     *sdktrace.TracerProvider
	tracer trace.Tracer

	mu   sync.Mutex
	runs map[string]trace.Span // runID -> root span
}

// Start wires an OTLP HTTP exporter when cfg.Endpoint is non-empty.
// Returns nil exporter (no-op) when disabled.
func Start(ctx context.Context, cfg Config) (*Exporter, error) {
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		return nil, nil
	}
	service := strings.TrimSpace(cfg.ServiceName)
	if service == "" {
		service = "baize"
	}
	opts := []otlptracehttp.Option{otlptracehttp.WithEndpointURL(endpoint)}
	if strings.HasPrefix(strings.ToLower(endpoint), "http://") {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exp, err := otlptracehttp.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(service),
		),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return &Exporter{
		tp:     tp,
		tracer: tp.Tracer("baize"),
		runs:   make(map[string]trace.Span),
	}, nil
}

// StartWithProvider is for tests (in-memory / span recorder).
func StartWithProvider(tp *sdktrace.TracerProvider) *Exporter {
	if tp == nil {
		return nil
	}
	otel.SetTracerProvider(tp)
	return &Exporter{
		tp:     tp,
		tracer: tp.Tracer("baize"),
		runs:   make(map[string]trace.Span),
	}
}

// AttachHub subscribes to run events and terminal status.
func (e *Exporter) AttachHub(hub *eventbus.Hub) {
	if e == nil || hub == nil {
		return
	}
	hub.OnEvent(func(runID string, ev eventbus.IndexedEvent) {
		e.onEvent(runID, ev)
	})
	hub.OnEnd(func(runID string, status store.Status) {
		e.onEnd(runID, status)
	})
}

// Shutdown flushes and closes the provider.
func (e *Exporter) Shutdown(ctx context.Context) error {
	if e == nil || e.tp == nil {
		return nil
	}
	e.mu.Lock()
	for id, sp := range e.runs {
		sp.End()
		delete(e.runs, id)
	}
	e.mu.Unlock()
	return e.tp.Shutdown(ctx)
}

func (e *Exporter) onEvent(runID string, ev eventbus.IndexedEvent) {
	if e == nil {
		return
	}
	switch ev.Event.Type {
	case "run.started":
		_, sp := e.tracer.Start(context.Background(), "baize.run",
			trace.WithAttributes(attribute.String("baize.run_id", runID)),
		)
		e.mu.Lock()
		if old, ok := e.runs[runID]; ok {
			old.End()
		}
		e.runs[runID] = sp
		e.mu.Unlock()
	case "tool.result":
		e.spanToolResult(runID, ev.Event.Data)
	}
}

func (e *Exporter) onEnd(runID string, status store.Status) {
	if e == nil {
		return
	}
	e.mu.Lock()
	sp, ok := e.runs[runID]
	delete(e.runs, runID)
	e.mu.Unlock()
	if !ok {
		return
	}
	sp.SetAttributes(attribute.String("baize.run_status", string(status)))
	if status == store.StatusFailed || status == store.StatusCancelled || status == store.StatusRejected {
		sp.SetStatus(codes.Error, string(status))
	}
	sp.End()
}

func (e *Exporter) spanToolResult(runID string, data map[string]any) {
	ctx := context.Background()
	e.mu.Lock()
	if parent, ok := e.runs[runID]; ok {
		ctx = trace.ContextWithSpan(ctx, parent)
	}
	e.mu.Unlock()

	name, _ := data["name"].(string)
	if name == "" {
		name = "tool"
	}
	attrs := []attribute.KeyValue{
		attribute.String("baize.run_id", runID),
		attribute.String("baize.tool", name),
	}
	if cid, ok := data["connector_id"].(string); ok && cid != "" {
		attrs = append(attrs, attribute.String("baize.connector_id", cid))
	}
	if kid, ok := data["export_key_id"].(string); ok && kid != "" {
		attrs = append(attrs, attribute.String("baize.export_key_id", kid))
	}
	if iid, ok := data["identity_id"].(string); ok && iid != "" {
		attrs = append(attrs, attribute.String("baize.identity_id", iid))
	}
	_, sp := e.tracer.Start(ctx, "baize.tool "+name, trace.WithAttributes(attrs...))
	if isErr, _ := data["is_error"].(bool); isErr {
		sp.SetStatus(codes.Error, "tool error")
	}
	sp.End()
}

// RecordExportCall emits a standalone span for MCP export tool invocations
// (no Chat run). Safe when e is nil.
func (e *Exporter) RecordExportCall(toolName, exportKeyID, identityID, contractHash string, catalogRevision int64, isError bool) {
	if e == nil {
		return
	}
	attrs := []attribute.KeyValue{
		attribute.String("baize.tool", toolName),
		attribute.String("baize.channel", "mcp_export"),
	}
	if exportKeyID != "" {
		attrs = append(attrs, attribute.String("baize.export_key_id", exportKeyID))
	}
	if identityID != "" {
		attrs = append(attrs, attribute.String("baize.identity_id", identityID))
	}
	if contractHash != "" {
		attrs = append(attrs, attribute.String("baize.contract_hash", contractHash))
	}
	if catalogRevision > 0 {
		attrs = append(attrs, attribute.Int64("baize.catalog_revision", catalogRevision))
	}
	_, sp := e.tracer.Start(context.Background(), "baize.mcp_export "+toolName,
		trace.WithAttributes(attrs...))
	if isError {
		sp.SetStatus(codes.Error, "tool error")
	}
	sp.End()
}

// MustAttach is like AttachHub but logs Start failures from the caller.
func LogStartError(err error) {
	if err != nil {
		applog.L().Warn("disable export", slog.String("component", "otelx"), slog.Any("err", err))
	}
}
