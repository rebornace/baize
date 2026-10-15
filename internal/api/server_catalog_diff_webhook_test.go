package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/webhook"
)

const catalogWebhookSpecV1 = `openapi: 3.0.3
info:
  title: catalog-webhook
  version: 0.1.0
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses:
        "201":
          description: created
  /tickets/{id}:
    get:
      operationId: get_ticket
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: ok
`

const catalogWebhookSpecV2 = `openapi: 3.0.3
info:
  title: catalog-webhook
  version: 0.1.0
paths:
  /tickets:
    post:
      operationId: create_ticket
      responses:
        "201":
          description: created
  /health:
    get:
      operationId: health_check
      responses:
        "200":
          description: ok
`

func TestPutConnectorNotifyCatalogDiffWebhook(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	hook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		mu.Lock()
		bodies = append(bodies, body)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer hook.Close()

	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := NewServer(st, reg, &gateFakeRunner{store: st})
	srv.Identities = identity.NewMemoryStore()
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatalf("open memory blob: %v", err)
	}
	srv.Blobs = blobs

	d := webhook.NewDispatcher(st, webhook.Config{URL: hook.URL})
	srv.Webhook = d
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.StartWorker(ctx)
	h := srv.Handler()

	putOpenAPI := func(specContent string) {
		t.Helper()
		body := map[string]any{
			"type":          "openapi",
			"base_url":      "http://127.0.0.1:9",
			"spec_content":  specContent,
			"import_format": "openapi3",
		}
		req := httptest.NewRequest(http.MethodPut, "/v0/connectors/c-wh", jsonBodyAPI(t, body))
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("PUT connector status=%d body=%s", rr.Code, rr.Body.String())
		}
	}

	putOpenAPI(catalogWebhookSpecV1)
	putOpenAPI(catalogWebhookSpecV2)

	deadline := time.Now().Add(3 * time.Second)
	for {
		mu.Lock()
		n := len(bodies)
		mu.Unlock()
		if n >= 2 || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	var sawDiff bool
	for _, b := range bodies {
		if b["type"] != "catalog.diff" {
			continue
		}
		if b["connector_id"] != "c-wh" {
			t.Fatalf("connector_id=%v", b["connector_id"])
		}
		diff, _ := b["diff"].(map[string]any)
		if diff == nil {
			t.Fatalf("missing diff in body=%+v", b)
		}
		removed, _ := diff["removed"].([]any)
		added, _ := diff["added"].([]any)
		hasGetRemoved := false
		for _, x := range removed {
			if x == "get_ticket" {
				hasGetRemoved = true
			}
		}
		hasHealthAdded := false
		for _, x := range added {
			if x == "health_check" {
				hasHealthAdded = true
			}
		}
		if hasGetRemoved && hasHealthAdded {
			sawDiff = true
			break
		}
	}
	if !sawDiff {
		t.Fatalf("no catalog.diff with get_ticket removed and health_check added; bodies=%+v", bodies)
	}
}
