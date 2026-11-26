package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

func TestOperatorLimitsAPIAndEnforcement(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	h := srv.Handler()

	put := httptest.NewRequest(http.MethodPut, "/v0/settings/operator-limits",
		bytes.NewBufferString(`{"requests_per_minute":1,"runs_per_day":1,"max_concurrent_runs":0}`))
	put.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, put)
	if rr.Code != http.StatusOK {
		t.Fatalf("put=%d %s", rr.Code, rr.Body.String())
	}
	var body operatorLimits
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil || body.RunsPerDay != 1 {
		t.Fatalf("body=%+v err=%v", body, err)
	}

	ctx := controlplane.WithRole(context.Background(), controlplane.RoleOperator)
	ctx = controlplane.WithOperatorID(ctx, "rate-test")
	if !srv.allowOperatorRequest(ctx) {
		t.Fatal("first request should pass")
	}
	if srv.allowOperatorRequest(ctx) {
		t.Fatal("second request should be rate limited")
	}

	// Daily run quota: one existing run today owned by the operator.
	ms := conversation.NewMemoryStore()
	srv.Messages = ms
	if err := ms.EnsureMeta(conversation.Meta{ID: "c-limit", OwnerID: "limit-op", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	r, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "x", ConversationID: "c-limit",
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = r
	if err := srv.Store.UpsertSetting(operatorLimitsKey,
		[]byte(`{"requests_per_minute":0,"runs_per_day":1,"max_concurrent_runs":0}`)); err != nil {
		t.Fatal(err)
	}
	opCtx := controlplane.WithRole(context.Background(), controlplane.RoleOperator)
	opCtx = controlplane.WithOperatorID(opCtx, "limit-op")
	if err := srv.checkOperatorRunLimits(opCtx); err != ErrOperatorRunQuota {
		t.Fatalf("run quota err=%v", err)
	}

	// Concurrent quota: the queued run counts as active.
	if err := srv.Store.UpsertSetting(operatorLimitsKey,
		[]byte(`{"requests_per_minute":0,"runs_per_day":0,"max_concurrent_runs":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := srv.checkOperatorRunLimits(opCtx); err != ErrOperatorConcurrency {
		t.Fatalf("concurrency err=%v", err)
	}
}
