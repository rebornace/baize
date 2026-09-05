package api_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rebornace/baize/internal/api"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workspace"
)

// failingWorkspace is an UploadSaver whose every save fails, used to assert
// that workspace persistence errors never block the run.
type failingWorkspace struct{}

func (failingWorkspace) SaveUpload(context.Context, string, string, string) (string, error) {
	return "", errors.New("boom")
}
func (failingWorkspace) SaveUploadBytes(context.Context, string, string, []byte, string) (string, error) {
	return "", errors.New("boom")
}

func TestPostRunPersistsTextAttachmentToWorkspace(t *testing.T) {
	mem := store.NewMemory()
	mem.UpsertAgent(store.Agent{ID: "a", System: "s"})
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})

	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv.Workspace = workspace.New(blobs)
	h := srv.Handler()

	body := map[string]any{
		"agent_id":        "a",
		"input":           "summarize this",
		"conversation_id": "conv1",
		"attachments": []map[string]any{{
			"filename":       "note.txt",
			"media_type":     "text/plain",
			"content_base64": base64.StdEncoding.EncodeToString([]byte("hello file contents")),
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}

	got, err := blobs.Get(context.Background(), "workspaces/conv1/uploads/note.txt")
	if err != nil {
		t.Fatalf("attachment not persisted: %v", err)
	}
	if string(got) != "hello file contents" {
		t.Fatalf("persisted content=%q", got)
	}
}

func TestPostRunPersistsImageAttachmentToWorkspace(t *testing.T) {
	mem := store.NewMemory()
	mem.UpsertAgent(store.Agent{ID: "a", System: "s"})
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})
	// Reuse the vision-capable stub from server_attachments_test.go so the
	// image attachment passes the supports_vision gate.
	srv.LLM = &captureUserLLM{vision: true}

	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	srv.Workspace = workspace.New(blobs, workspace.WithVision(func() bool { return true }))
	h := srv.Handler()

	body := map[string]any{
		"agent_id":        "a",
		"input":           "look",
		"conversation_id": "conv1",
		"attachments": []map[string]any{{
			"filename":   "shot.png",
			"media_type": "image/png",
			// Reuse the valid-PNG helper from server_attachments_test.go (a
			// real encodable 2x2 PNG); attach re-encodes the thumbnail but
			// keeps the filename, so the blob key stays uploads/shot.png.
			"content_base64": tinyPNGBase64(t),
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if _, err := blobs.Get(context.Background(), "workspaces/conv1/uploads/shot.png"); err != nil {
		t.Fatalf("image not persisted: %v", err)
	}
}

func TestPostRunWorkspaceFailureDoesNotBlock(t *testing.T) {
	mem := store.NewMemory()
	mem.UpsertAgent(store.Agent{ID: "a", System: "s"})
	reg := tool.NewRegistry()
	srv := api.NewServer(mem, reg, &fakeRunner{store: mem})
	srv.Workspace = failingWorkspace{}
	h := srv.Handler()

	body := map[string]any{
		"agent_id":        "a",
		"input":           "hi",
		"conversation_id": "conv1",
		"attachments": []map[string]any{{
			"filename":       "note.txt",
			"media_type":     "text/plain",
			"content_base64": base64.StdEncoding.EncodeToString([]byte("x")),
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "/v0/runs", jsonBody(t, body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("workspace failure must not block run: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["run_id"] == nil || resp["run_id"] == "" {
		t.Fatalf("run should still be created: %v", resp)
	}
}
