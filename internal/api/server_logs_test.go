package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/applog"
)

func TestRecentLogsRequiresAdmin(t *testing.T) {
	applog.ResetRingForTest()
	applog.InitWithWriter("text", &strings.Builder{})
	applog.L().Info("admin-logs-probe", slog.String("component", "test"))

	srv := gateServer(t, "op", "adm")
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v0/settings/logs", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("no auth: code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/settings/logs", nil)
	req.Header.Set("Authorization", "Bearer op")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("operator: code=%d body=%s", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/v0/settings/logs?limit=50", nil)
	req.Header.Set("Authorization", "Bearer adm")
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admin: code=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Lines []string `json:"lines"`
		Limit int      `json:"limit"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Limit != 50 {
		t.Fatalf("limit=%d", body.Limit)
	}
	joined := strings.Join(body.Lines, "\n")
	if !strings.Contains(joined, "admin-logs-probe") {
		t.Fatalf("missing probe line: %v", body.Lines)
	}
}
