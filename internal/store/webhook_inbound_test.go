package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func webhookInboundCase(t *testing.T, st Store) {
	t.Helper()
	now := time.Now().UTC().Add(time.Second)
	job := WebhookInboundJob{
		DedupeKey: "feishu:k1",
		Channel:   "feishu",
		Body:      []byte(`{"text":"hi"}`),
	}
	created, id, err := st.PutWebhookInboundJobIfAbsent(job)
	if err != nil || !created || id == "" {
		t.Fatalf("put: created=%v id=%q err=%v", created, id, err)
	}
	dup, dupID, err := st.PutWebhookInboundJobIfAbsent(job)
	if err != nil || dup || dupID != id {
		t.Fatalf("dedupe: created=%v id=%q err=%v", dup, dupID, err)
	}
	due, err := st.ListWebhookInboundJobsDue(now, 10)
	if err != nil || len(due) != 1 || due[0].Body == nil || due[0].Status != WebhookInboundPending {
		t.Fatalf("due=%+v err=%v", due, err)
	}

	// A future next_attempt_at is not due yet.
	job = due[0]
	job.NextAttemptAt = now.Add(time.Minute)
	if err := st.UpdateWebhookInboundJob(job); err != nil {
		t.Fatal(err)
	}
	due, err = st.ListWebhookInboundJobsDue(now, 10)
	if err != nil || len(due) != 0 {
		t.Fatalf("future due=%+v err=%v", due, err)
	}
	due, err = st.ListWebhookInboundJobsDue(now.Add(2*time.Minute), 10)
	if err != nil || len(due) != 1 {
		t.Fatalf("later due=%+v err=%v", due, err)
	}

	// Done jobs are no longer due but still dedupe until cleanup.
	job = due[0]
	job.Status = WebhookInboundDone
	if err := st.UpdateWebhookInboundJob(job); err != nil {
		t.Fatal(err)
	}
	due, _ = st.ListWebhookInboundJobsDue(now.Add(3*time.Minute), 10)
	if len(due) != 0 {
		t.Fatalf("done still due: %+v", due)
	}
	dup, _, err = st.PutWebhookInboundJobIfAbsent(WebhookInboundJob{DedupeKey: job.DedupeKey})
	if err != nil || dup {
		t.Fatalf("done dedupe: created=%v err=%v", dup, err)
	}

	// Dead-letter: list/get/reset.
	_, deadID, err := st.PutWebhookInboundJobIfAbsent(WebhookInboundJob{
		DedupeKey: "feishu:k2", Channel: "feishu", Body: []byte(`{"text":"dead"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	due, err = st.ListWebhookInboundJobsDue(now.Add(3*time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	var deadJob WebhookInboundJob
	for _, j := range due {
		if j.ID == deadID {
			deadJob = j
		}
	}
	if deadJob.ID == "" {
		t.Fatalf("dead job not due: %+v", due)
	}
	deadJob.Status = WebhookInboundDead
	deadJob.Attempt = 3
	deadJob.LastError = "boom"
	if err := st.UpdateWebhookInboundJob(deadJob); err != nil {
		t.Fatal(err)
	}
	rows, err := st.ListWebhookInboundJobs("feishu", []WebhookInboundStatus{WebhookInboundDead}, 10)
	if err != nil || len(rows) != 1 || rows[0].ID != deadID || rows[0].LastError != "boom" {
		t.Fatalf("dead rows=%+v err=%v", rows, err)
	}
	got, ok, err := st.GetWebhookInboundJob(deadID)
	if err != nil || !ok || got.Status != WebhookInboundDead {
		t.Fatalf("get=%+v ok=%v err=%v", got, ok, err)
	}
	if err := st.ResetWebhookInboundJob(deadID); err != nil {
		t.Fatal(err)
	}
	got, ok, _ = st.GetWebhookInboundJob(deadID)
	if !ok || got.Status != WebhookInboundPending || got.Attempt != 0 || got.LastError != "" {
		t.Fatalf("reset=%+v", got)
	}

	if err := st.DeleteWebhookInboundJobsBefore(now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	created, _, err = st.PutWebhookInboundJobIfAbsent(WebhookInboundJob{DedupeKey: job.DedupeKey})
	if err != nil || !created {
		t.Fatalf("after cleanup: created=%v err=%v", created, err)
	}
}

func TestWebhookInboundJobsMemory(t *testing.T) {
	webhookInboundCase(t, NewMemory())
}

func TestWebhookInboundJobsSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webhook_inbound.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	webhookInboundCase(t, st)
}

func webhookInboundClaimCase(t *testing.T, st Store) {
	t.Helper()
	now := time.Now().UTC()
	created, id, err := st.PutWebhookInboundJobIfAbsent(WebhookInboundJob{
		DedupeKey:     "feishu:claim-" + now.Format(time.RFC3339Nano),
		Channel:       "feishu",
		Body:          []byte(`{"text":"claim"}`),
		NextAttemptAt: now.Add(-time.Second),
	})
	if err != nil || !created {
		t.Fatalf("put: created=%v err=%v", created, err)
	}
	job, ok, err := st.ClaimWebhookInboundJob(now, "w1", time.Minute)
	if err != nil || !ok || job.ID != id || job.ClaimedBy != "w1" {
		t.Fatalf("claim w1: %+v ok=%v err=%v", job, ok, err)
	}
	if _, ok, err := st.ClaimWebhookInboundJob(now, "w2", time.Minute); err != nil || ok {
		t.Fatalf("claim w2 while leased: ok=%v err=%v", ok, err)
	}
	reclaimed, ok, err := st.ClaimWebhookInboundJob(now.Add(2*time.Minute), "w2", time.Minute)
	if err != nil || !ok || reclaimed.ClaimedBy != "w2" {
		t.Fatalf("reclaim after lease: %+v ok=%v err=%v", reclaimed, ok, err)
	}
	if err := st.UpdateWebhookInboundJob(job); err == nil {
		t.Fatal("stale claim update must fail")
	}
	reclaimed.Status = WebhookInboundDone
	if err := st.UpdateWebhookInboundJob(reclaimed); err != nil {
		t.Fatalf("finish: %v", err)
	}
	got, ok, err := st.GetWebhookInboundJob(id)
	if err != nil || !ok || got.Status != WebhookInboundDone || got.ClaimedBy != "" || !got.ClaimExpiresAt.IsZero() {
		t.Fatalf("after finish: %+v ok=%v err=%v", got, ok, err)
	}
}

func TestWebhookInboundClaimMemory(t *testing.T) {
	webhookInboundClaimCase(t, NewMemory())
}

func TestWebhookInboundClaimSQLite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "webhook_claim.db")
	st, err := OpenSQLite(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	webhookInboundClaimCase(t, st)
}

func TestWebhookInboundClaimPostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	webhookInboundClaimCase(t, st)
}
