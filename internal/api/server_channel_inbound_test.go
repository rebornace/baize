package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func inboundJobsServer(t *testing.T) (*Server, *store.Memory, string) {
	t.Helper()
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	srv.AdminToken = "adm-token"
	srv.Operators = []controlplane.Operator{{ID: "alice", Token: "alice-token"}}

	created, id, err := st.PutWebhookInboundJobIfAbsent(store.WebhookInboundJob{
		DedupeKey: "feishu:k1",
		Channel:   "feishu",
		Body:      []byte(`{"text":"secret inbound body"}`),
	})
	if err != nil || !created {
		t.Fatalf("put job: created=%v id=%q err=%v", created, id, err)
	}
	job, ok, err := st.GetWebhookInboundJob(id)
	if err != nil || !ok {
		t.Fatalf("get job: %v %v", ok, err)
	}
	job.Status = store.WebhookInboundDead
	job.Attempt = 3
	job.LastError = "attachment timeout"
	if err := st.UpdateWebhookInboundJob(job); err != nil {
		t.Fatal(err)
	}
	return srv, st, id
}

func TestChannelInboundJobsListAndRetry(t *testing.T) {
	srv, st, id := inboundJobsServer(t)
	h := srv.Handler()

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withNamedOp(
		httptest.NewRequest(http.MethodGet, "/v0/settings/channels/inbound-jobs", nil), "adm-token"))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		Jobs []channelInboundJobJSON `json:"jobs"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Jobs) != 1 || body.Jobs[0].ID != id || body.Jobs[0].LastError != "attachment timeout" {
		t.Fatalf("jobs=%+v", body.Jobs)
	}
	if body.Jobs[0].Channel != "feishu" || body.Jobs[0].Attempt != 3 {
		t.Fatalf("job=%+v", body.Jobs[0])
	}
	if strings.Contains(rec.Body.String(), "secret inbound body") {
		t.Fatalf("response leaked body: %s", rec.Body.String())
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, withNamedOp(
		httptest.NewRequest(http.MethodPost, "/v0/settings/channels/inbound-jobs/"+id+"/retry", nil),
		"adm-token"))
	if rec.Code != http.StatusOK {
		t.Fatalf("retry status=%d body=%s", rec.Code, rec.Body.String())
	}
	job, ok, err := st.GetWebhookInboundJob(id)
	if err != nil || !ok {
		t.Fatalf("get after retry: %v %v", ok, err)
	}
	if job.Status != store.WebhookInboundPending || job.Attempt != 0 || job.LastError != "" {
		t.Fatalf("job after retry=%+v", job)
	}
	due, err := st.ListWebhookInboundJobsDue(time.Now().UTC().Add(time.Second), 10)
	if err != nil || len(due) != 1 || due[0].ID != id {
		t.Fatalf("due=%+v err=%v", due, err)
	}
}

func TestChannelInboundJobsRetryNotFound(t *testing.T) {
	srv, _, _ := inboundJobsServer(t)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, withNamedOp(
		httptest.NewRequest(http.MethodPost, "/v0/settings/channels/inbound-jobs/nope/retry", nil),
		"adm-token"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
