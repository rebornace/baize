package api_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	deletedConv        string
	deletedObject      string
	deleteErr          error
}

func (f *fakeChannelMedia) OpenMedia(_ context.Context, conv, object string) ([]byte, string, bool, error) {
	f.gotConv, f.gotObject = conv, object
	return f.data, f.mime, f.found, f.err
}

func (f *fakeChannelMedia) DeleteMedia(_ context.Context, conv, object string) error {
	f.deletedConv, f.deletedObject = conv, object
	return f.deleteErr
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

type recordingWorkspace struct {
	deletedConv string
	deletedName string
}

func (recordingWorkspace) SaveUpload(context.Context, string, string, string) (string, error) {
	return "", nil
}
func (recordingWorkspace) SaveUploadBytes(context.Context, string, string, []byte, string) (string, error) {
	return "", nil
}
func (w *recordingWorkspace) DeleteUpload(_ context.Context, conv, name string) error {
	w.deletedConv, w.deletedName = conv, name
	return nil
}

func deleteMediaServer(t *testing.T, media api.ChannelMediaOpener, ws api.UploadSaver) http.Handler {
	t.Helper()
	mem := store.NewMemory()
	srv := api.NewServer(mem, tool.NewRegistry(), &fakeRunner{store: mem})
	srv.ChannelMedia = media
	srv.Workspace = ws
	return srv.Handler()
}

func TestChannelMediaDeleteRemovesMediaAndWorkspaceCopy(t *testing.T) {
	media := &fakeChannelMedia{}
	ws := &recordingWorkspace{}
	h := deleteMediaServer(t, media, ws)

	name := url.QueryEscape("报告.pdf")
	req := httptest.NewRequest(http.MethodDelete,
		"/v0/channels/media/weixin:acc:peer/abc-123.pdf?name="+name, nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if media.deletedConv != "weixin:acc:peer" || media.deletedObject != "abc-123.pdf" {
		t.Fatalf("deleted conv=%q object=%q", media.deletedConv, media.deletedObject)
	}
	if ws.deletedConv != "weixin:acc:peer" || ws.deletedName != "报告.pdf" {
		t.Fatalf("workspace delete conv=%q name=%q", ws.deletedConv, ws.deletedName)
	}
}

func TestChannelMediaDeleteWithoutNameSkipsWorkspace(t *testing.T) {
	media := &fakeChannelMedia{}
	ws := &recordingWorkspace{}
	h := deleteMediaServer(t, media, ws)

	req := httptest.NewRequest(http.MethodDelete, "/v0/channels/media/c/abc.png", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if ws.deletedName != "" {
		t.Fatalf("workspace delete called with %q", ws.deletedName)
	}
}

func TestChannelMediaDeleteRejectsTraversal(t *testing.T) {
	media := &fakeChannelMedia{}
	h := deleteMediaServer(t, media, nil)

	req := httptest.NewRequest(http.MethodDelete, "/v0/channels/media/c/a..b", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}
	if media.deletedObject != "" {
		t.Fatalf("delete reached store with object=%q", media.deletedObject)
	}
}
