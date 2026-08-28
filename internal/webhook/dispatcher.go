package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/eventbus"
	"github.com/rebornace/baize/internal/store"
)

const (
	protocolHeader = "X-Baize-Protocol"
	protocolValue  = "v0"
	runIDHeader    = "X-Baize-Run-Id"
	postTimeout    = 30 * time.Second
)

// Dispatcher delivers run events to configured webhook URLs asynchronously.
type Dispatcher struct {
	mu     sync.RWMutex
	cfg    Config
	store  store.Store
	client *http.Client
	wake   chan struct{}
}

// NewDispatcher creates a webhook dispatcher with the given initial config.
func NewDispatcher(st store.Store, cfg Config) *Dispatcher {
	return &Dispatcher{
		cfg:   cfg,
		store: st,
		client: &http.Client{
			Timeout: postTimeout,
		},
		wake: make(chan struct{}, 1),
	}
}

// Attach registers hub observers for asynchronous event delivery.
func (d *Dispatcher) Attach(hub *eventbus.Hub) {
	hub.OnEvent(func(runID string, ev eventbus.IndexedEvent) {
		d.dispatchEvent(runID, ev)
	})
	hub.OnEnd(func(runID string, status store.Status) {
		d.dispatchEnd(runID, status)
	})
}

// StartWorker processes due outbox rows until ctx is cancelled.
func (d *Dispatcher) StartWorker(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.wake:
			d.processDue(ctx)
		case <-ticker.C:
			d.processDue(ctx)
		}
	}
}

// UpdateConfig hot-reloads the global webhook configuration.
func (d *Dispatcher) UpdateConfig(cfg Config) {
	d.mu.Lock()
	d.cfg = cfg
	d.mu.Unlock()
}

// Config returns the current global webhook configuration.
func (d *Dispatcher) Config() Config {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.cfg
}

// SendTest enqueues a webhook.test delivery and waits for the first POST attempt.
func (d *Dispatcher) SendTest(ctx context.Context) error {
	url, headers, err := d.resolveTarget("", nil)
	if err != nil {
		return err
	}
	if strings.TrimSpace(url) == "" {
		return ErrNoURL
	}
	payload := map[string]any{
		"run_id": "test",
		"index":  0,
		"event": map[string]any{
			"type":      "webhook.test",
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
			"data":      map[string]any{},
		},
	}
	id, created, err := d.enqueue(ctx, "test", store.WebhookOutboxKindEvent, 0, url, headers, payload, false)
	if err != nil {
		return err
	}
	if !created {
		entry, err := d.store.GetWebhookOutbox(id)
		if err != nil {
			return err
		}
		if entry.Status == store.WebhookOutboxDelivered {
			return nil
		}
	}
	entry, err := d.store.GetWebhookOutbox(id)
	if err != nil {
		return err
	}
	d.deliverOne(ctx, entry)
	entry, err = d.store.GetWebhookOutbox(id)
	if err != nil {
		return err
	}
	if entry.Status == store.WebhookOutboxDelivered {
		return nil
	}
	if entry.LastError != "" {
		return errors.New(entry.LastError)
	}
	return ErrDeliveryFailed
}

func (d *Dispatcher) dispatchEvent(runID string, ev eventbus.IndexedEvent) {
	run, err := d.store.GetRun(runID)
	if err != nil {
		return
	}
	url, headers, err := d.resolveTarget(runID, run.WebhookConfig)
	if err != nil || strings.TrimSpace(url) == "" {
		return
	}
	payload := map[string]any{
		"run_id": runID,
		"index":  ev.Index,
		"event":  ev.Event,
	}
	_, _, _ = d.enqueue(context.Background(), runID, store.WebhookOutboxKindEvent, ev.Index, url, headers, payload, true)
}

func (d *Dispatcher) dispatchEnd(runID string, status store.Status) {
	if status != store.StatusSucceeded && status != store.StatusFailed {
		return
	}
	run, err := d.store.GetRun(runID)
	if err != nil {
		return
	}
	url, headers, err := d.resolveTarget(runID, run.WebhookConfig)
	if err != nil || strings.TrimSpace(url) == "" {
		return
	}
	payload := map[string]any{
		"run_id": runID,
		"ended":  true,
		"status": string(status),
	}
	_, _, _ = d.enqueue(context.Background(), runID, store.WebhookOutboxKindEnded, -1, url, headers, payload, true)
}

func (d *Dispatcher) enqueue(ctx context.Context, runID string, kind store.WebhookOutboxKind, eventIndex int, url string, headers map[string]string, payload any, notifyWorker bool) (string, bool, error) {
	_ = ctx
	body, err := json.Marshal(payload)
	if err != nil {
		return "", false, err
	}
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return "", false, err
	}
	entry := store.WebhookOutboxEntry{
		RunID:       runID,
		Kind:        kind,
		EventIndex:  eventIndex,
		PayloadJSON: body,
		TargetURL:   url,
		HeadersJSON: headersJSON,
	}
	created, id, err := d.store.PutWebhookOutboxIfAbsent(entry)
	if err != nil {
		return "", false, err
	}
	if created && notifyWorker {
		d.signalWake()
	}
	return id, created, nil
}

func (d *Dispatcher) signalWake() {
	select {
	case d.wake <- struct{}{}:
	default:
	}
}

func (d *Dispatcher) processDue(ctx context.Context) {
	entries, err := d.store.ListWebhookOutboxDue(time.Now().UTC(), 20)
	if err != nil {
		return
	}
	for _, entry := range entries {
		d.deliverOne(ctx, entry)
	}
}

func (d *Dispatcher) deliverOne(ctx context.Context, entry store.WebhookOutboxEntry) {
	headers, err := store.DecodeWebhookOutboxHeaders(entry.HeadersJSON)
	if err != nil {
		entry.Attempt++
		entry.Status = store.WebhookOutboxDead
		entry.LastError = err.Error()
		_ = d.store.UpdateWebhookOutbox(entry)
		d.maybeEmitDeadEvent(entry)
		return
	}

	statusCode, postErr := d.post(ctx, entry.TargetURL, headers, entry.RunID, entry.PayloadJSON)
	if postErr == nil && statusCode >= 200 && statusCode < 300 {
		entry.Status = store.WebhookOutboxDelivered
		entry.LastError = ""
		_ = d.store.UpdateWebhookOutbox(entry)
		return
	}

	entry.Attempt++
	entry.LastError = formatDeliveryError(statusCode, postErr)
	if !Retryable(statusCode, postErr) || entry.Attempt >= entry.MaxAttempts {
		entry.Status = store.WebhookOutboxDead
		_ = d.store.UpdateWebhookOutbox(entry)
		d.maybeEmitDeadEvent(entry)
		return
	}
	entry.NextRetryAt = time.Now().UTC().Add(Backoff(entry.Attempt))
	_ = d.store.UpdateWebhookOutbox(entry)
}

func (d *Dispatcher) maybeEmitDeadEvent(entry store.WebhookOutboxEntry) {
	if entry.RunID == "" || entry.RunID == "test" {
		return
	}
	_ = d.store.AppendEvent(entry.RunID, store.Event{
		Type:      "webhook.delivery_dead",
		Timestamp: time.Now().UTC(),
		Data: map[string]any{
			"delivery_id": entry.ID,
			"kind":        string(entry.Kind),
			"event_index": entry.EventIndex,
			"attempt":     entry.Attempt,
			"error":       entry.LastError,
		},
	})
}

// RetryDelivery resets a dead/pending row for manual replay.
func (d *Dispatcher) RetryDelivery(id string) error {
	if err := d.store.ResetWebhookOutboxRetry(id); err != nil {
		return err
	}
	d.signalWake()
	return nil
}

func (d *Dispatcher) resolveTarget(runID string, perRun *store.WebhookConfig) (string, map[string]string, error) {
	_ = runID
	d.mu.RLock()
	global := d.cfg
	d.mu.RUnlock()

	url := strings.TrimSpace(global.URL)
	headers := cloneHeaders(global.Headers)
	if perRun != nil {
		if u := strings.TrimSpace(perRun.URL); u != "" {
			url = u
		}
		if len(perRun.Headers) > 0 {
			headers = cloneHeaders(perRun.Headers)
		}
	}
	resolved, err := resolveHeaders(headers)
	if err != nil {
		return "", nil, err
	}
	return url, resolved, nil
}

func (d *Dispatcher) post(ctx context.Context, url string, headers map[string]string, runID string, payload []byte) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(protocolHeader, protocolValue)
	req.Header.Set(runIDHeader, runID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, nil
}

func cloneHeaders(h map[string]string) map[string]string {
	if h == nil {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = v
	}
	return out
}
