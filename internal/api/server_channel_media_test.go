package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// fakeChannelMedia is a test ChannelMediaOpener.
type fakeChannelMedia struct {
	data               []byte
	mime               string
	found              bool
	err                error
	gotConv, gotObject string
}

func (f *fakeChannelMedia) OpenMedia(_ context.Context, conv, object string) ([]byte, string, bool, error) {
	f.gotConv, f.gotObject = conv, object
	return f.data, f.mime, f.found, f.err
}

func mediaServer(t *testing.T, opener api.ChannelMediaOpener) (*api.Server, http.Handler) {
	t.Helper()
	mem := store.NewMemory()
	srv := api.NewServer(mem, tool.NewRegistry(), &fakeRunner{store: mem})
	srv.ChannelMedia = opener
	return srv, srv.Handler()
}

func TestChannelMediaServesImage(t *testing.T) {
	opener := &fakeChannelMedia{data: []byte{0x89, 'P', 'N', 'G'}, mime: "image/png", found: true}
	_, h := mediaServer(t, opener)

	req := httptest.NewRequest(http.MethodGet, "/v0/channels/media/weixin:acc:peer/abc-123.png", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("Content-Type = %q", ct)
	}
	if opener.gotConv != "weixin:acc:peer" || opener.gotObject != "abc-123.png" {
		t.Fatalf("opener got conv=%q object=%q", opener.gotConv, opener.gotObject)
	}
}

func TestChannelMediaNotFound(t *testing.T) {
	opener := &fakeChannelMedia{found: false}
	_, h := mediaServer(t, opener)

	req := httptest.NewRequest(http.MethodGet, "/v0/channels/media/c/missing.png", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestChannelMediaNotConfigured(t *testing.T) {
	mem := store.NewMemory()
	srv := api.NewServer(mem, tool.NewRegistry(), &fakeRunner{store: mem})
	h := srv.Handler()

	req := httptest.NewRequest(http.MethodGet, "/v0/channels/media/c/x.png", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 when media not configured", rr.Code)
	}
}
