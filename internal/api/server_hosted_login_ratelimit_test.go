package api_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestHostedLoginRateLimited(t *testing.T) {
	st := store.NewMemory()
	reg := tool.NewRegistry()
	srv := apiNewServer(st, reg)
	srv.HostedLoginLimiter = hostedlogin.NewRateLimiter(1, time.Minute)
	id, _, err := srv.HostedSessions.Issue(hostedlogin.Session{
		ConnectorID: "mall", Mode: hostedlogin.ModeTokenPaste, IdentityID: "mei_1",
		Fields: []store.HostedLoginField{{Name: "token", Arg: "token", Input: "password"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = st.UpsertMCPExportIdentity(store.MCPExportIdentity{ID: "mei_1", Name: "Ops"})
	h := srv.Handler()

	rr1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/"+id, nil)
	req1.RemoteAddr = "9.9.9.9:1234"
	h.ServeHTTP(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("first=%d", rr1.Code)
	}

	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/v0/auth/hosted/"+id, nil)
	req2.RemoteAddr = "9.9.9.9:1234"
	h.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Fatalf("second=%d body=%s", rr2.Code, rr2.Body.String())
	}
}
