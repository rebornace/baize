package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

func TestLoginEntriesAndInvokeRoutesRemoved(t *testing.T) {
	st := store.NewMemory()
	srv := NewServer(st, tool.NewRegistry(), &gateFakeRunner{store: st})
	h := srv.Handler()

	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/v0/conversations/c1/login-entries"},
		{http.MethodPost, "/v0/conversations/c1/login-invoke"},
	} {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(`{}`))
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusNotFound {
			t.Fatalf("%s %s: status=%d want 404", tc.method, tc.path, rr.Code)
		}
	}
}
