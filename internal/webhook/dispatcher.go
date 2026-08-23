package webhook

import (
	"bytes"
	"context"
	"encoding/json"
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
}

// NewDispatcher creates a webhook dispatcher with the given initial config.
func NewDispatcher(st store.Store, cfg Config) *Dispatcher {
	return &Dispatcher{
		cfg:   cfg,
		store: st,
		client: &http.Client{
			Timeout: postTimeout,
		},
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

// SendTest posts a webhook.test event to the configured URL.
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
	return d.post(ctx, url, headers, "test", payload)
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
	go func() {
		_ = d.post(context.Background(), url, headers, runID, payload)
	}()
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
	go func() {
		_ = d.post(context.Background(), url, headers, runID, payload)
	}()
}

func (d *Dispatcher) resolveTarget(runID string, perRun *store.WebhookConfig) (string, map[string]string, error) {
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

func (d *Dispatcher) post(ctx context.Context, url string, headers map[string]string, runID string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(protocolHeader, protocolValue)
	req.Header.Set(runIDHeader, runID)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ErrDeliveryFailed
	}
	return nil
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
