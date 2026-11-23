package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/cmd/weixin-adapter/internal/weixinlink"
)

func TestHealthz(t *testing.T) {
	a := &Adapter{ilink: weixinlink.NewFake()}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	a.handleHealthz(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz=%d", rec.Code)
	}
}
