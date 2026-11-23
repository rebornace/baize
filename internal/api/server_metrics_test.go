package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/metricsx"
	"github.com/rebornace/baize/internal/runtimecfg"
)

func metricsGateServer(t *testing.T) *Server {
	t.Helper()
	srv := gateServer(t, "op", "adm")
	m := metricsx.New()
	m.RunsTotal.WithLabelValues("succeeded").Inc()
	srv.MetricsHandler = m.Handler()
	base := runtimecfg.Snapshot{Creds: runtimecfg.Credentials{
		OperatorToken: "op",
		AdminToken:    "adm",
	}}
	srv.Settings = runtimecfg.New(base)
	return srv
}

func TestMetricsRequiresAdmin(t *testing.T) {
	srv := metricsGateServer(t)
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized || !strings.Contains(rr.Body.String(), "unauthorized") {
		t.Fatalf("no auth: code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden || !strings.Contains(rr.Body.String(), "forbidden") {
		t.Fatalf("operator: code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "baize_runs_total") {
		t.Fatalf("admin body missing baize_runs_total: %s", rr.Body.String())
	}
}

func TestMetricsNotRegisteredWhenHandlerNil(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("nil MetricsHandler: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestMetricsGateOffAllowsAnonymous(t *testing.T) {
	srv := gateServer(t, "", "")
	m := metricsx.New()
	m.RunsTotal.WithLabelValues("succeeded").Inc()
	srv.MetricsHandler = m.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !strings.Contains(rr.Body.String(), "baize_runs_total") {
		t.Fatalf("gate off: code=%d body=%s", rr.Code, rr.Body.String())
	}
}

// Ensure Settings-backed credentials gate /metrics the same as struct fields.
func TestMetricsUsesSettingsCredentials(t *testing.T) {
	srv := gateServer(t, "ignored", "ignored")
	m := metricsx.New()
	m.RunsTotal.WithLabelValues("succeeded").Inc()
	srv.MetricsHandler = m.Handler()
	base := runtimecfg.Snapshot{Creds: runtimecfg.Credentials{
		OperatorToken: "op2",
		AdminToken:    "adm2",
	}}
	h := runtimecfg.New(base)
	if err := h.ApplyCreds(context.Background(), nil, runtimecfg.CredsPatch{AdminToken: "adm2", OperatorToken: "op2"}); err != nil {
		t.Fatal(err)
	}
	srv.Settings = h

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer adm2")
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("settings admin: code=%d body=%s", rr.Code, rr.Body.String())
	}
}
