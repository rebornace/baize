package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/attach"
	"github.com/rebornace/baize/internal/conversation"
	"github.com/rebornace/baize/internal/store"
)

type hydrateFakeFile struct {
	data []byte
	mime string
}

type hydrateFakeMedia struct {
	data  []byte
	mime  string
	files map[string]hydrateFakeFile
}

func (f *hydrateFakeMedia) OpenMedia(_ context.Context, _ string, object string) ([]byte, string, bool, error) {
	if file, ok := f.files[object]; ok {
		return file.data, file.mime, true, nil
	}
	if len(f.data) == 0 {
		return nil, "", false, nil
	}
	return f.data, f.mime, true, nil
}

func (f *hydrateFakeMedia) DeleteMedia(context.Context, string, string) error { return nil }

func TestHydrateRunUserPartsFromMessage(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.Messages = conversation.NewMemoryStore()
	png := []byte{0x89, 'P', 'N', 'G', 0, 0, 0, 0}
	srv.ChannelMedia = &hydrateFakeMedia{data: png, mime: "image/png"}
	if _, err := srv.Messages.Append("c1", conversation.Message{
		Role:      conversation.RoleUser,
		RunID:     "r1",
		CreatedAt: time.Now().UTC(),
		Content:   "看图\n![图片](/v0/channels/media/c1/a.png?name=a.png)",
	}); err != nil {
		t.Fatal(err)
	}
	parts := srv.hydrateRunUserParts(context.Background(), "r1", "c1", "看图")
	if len(parts) != 2 {
		t.Fatalf("parts=%+v", parts)
	}
	if parts[0].Type != "text" || parts[0].Text != "看图" {
		t.Fatalf("text part=%+v", parts[0])
	}
	if parts[1].Type != "image" || parts[1].ImageMIME != "image/png" || len(parts[1].ImageBytes) != len(png) {
		t.Fatalf("image part=%+v", parts[1])
	}
	if got := srv.hydrateRunUserParts(context.Background(), "missing", "c1", "x"); got != nil {
		t.Fatalf("missing run parts=%+v", got)
	}
}

func TestHydrateRunUserPartsReplaysPDFFile(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.Messages = conversation.NewMemoryStore()
	profile, err := srv.Store.UpsertModelProfile(store.ModelProfile{
		Name: "pdf-replay", Model: "gpt-4o", SupportsPDF: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	pdf := []byte("%PDF-1.4 replay")
	srv.ChannelMedia = &hydrateFakeMedia{files: map[string]hydrateFakeFile{
		"scan.pdf": {data: pdf, mime: "application/pdf"},
	}}
	runRec, err := srv.Store.CreateRun(store.CreateRunInput{
		AgentID: "a", Input: "读附件", ConversationID: "c1", ModelProfileID: profile.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := srv.Messages.Append("c1", conversation.Message{
		Role:      conversation.RoleUser,
		RunID:     runRec.ID,
		CreatedAt: time.Now().UTC(),
		Content:   "读附件\n[file:scan.pdf](/v0/channels/media/c1/scan.pdf)",
	}); err != nil {
		t.Fatal(err)
	}
	parts := srv.hydrateRunUserParts(context.Background(), runRec.ID, "c1", "读附件")
	if len(parts) != 2 {
		t.Fatalf("parts=%+v", parts)
	}
	if parts[1].Type != "file" || parts[1].FileMIME != "application/pdf" ||
		parts[1].Filename != "scan.pdf" || len(parts[1].FileBytes) != len(pdf) {
		t.Fatalf("file part=%+v", parts[1])
	}
}

func TestAppendReplayImagesDedupesDocumentImages(t *testing.T) {
	doc := []byte("page-image-bytes")
	imgs := []attach.Extracted{
		{Kind: "image", ImageMIME: "image/png", ImageBytes: doc, Document: true},
		{Kind: "image", ImageMIME: "image/png", ImageBytes: doc, Document: true},
		{Kind: "image", ImageMIME: "image/png", ImageBytes: []byte("user-upload")},
	}
	seen := map[string]bool{documentImageKey("image/png", doc): true}
	parts := appendReplayImages(nil, imgs, seen)
	if len(parts) != 1 || string(parts[0].ImageBytes) != "user-upload" {
		t.Fatalf("parts=%+v", parts)
	}

	fresh := map[string]bool{}
	parts = appendReplayImages(nil, imgs, fresh)
	if len(parts) != 2 {
		t.Fatalf("fresh parts=%d, want one document image + one user image", len(parts))
	}
	if !fresh[documentImageKey("image/png", doc)] {
		t.Fatal("document image key was not recorded")
	}
	if !isDocumentImageURL("/v0/channels/media/c/p.jpg?name=scan.pdf&from=doc") {
		t.Fatal("from=doc URL not recognised")
	}
	if isDocumentImageURL("/v0/channels/media/c/p.jpg?name=from=doc.jpg") {
		t.Fatal("name query must not be mistaken for from=doc")
	}
}

func TestHydrateRunUserPartsIncludesTextAttachment(t *testing.T) {
	srv := gateServer(t, "op", "adm")
	srv.Messages = conversation.NewMemoryStore()
	srv.ChannelMedia = &hydrateFakeMedia{files: map[string]hydrateFakeFile{
		"notes.txt": {data: []byte("hello attachment"), mime: "text/plain"},
	}}
	if _, err := srv.Messages.Append("c1", conversation.Message{
		Role:      conversation.RoleUser,
		RunID:     "r2",
		CreatedAt: time.Now().UTC(),
		Content:   "看看附件\n[file:notes.txt](/v0/channels/media/c1/notes.txt)",
	}); err != nil {
		t.Fatal(err)
	}
	parts := srv.hydrateRunUserParts(context.Background(), "r2", "c1", "看看附件")
	if len(parts) != 1 {
		t.Fatalf("parts=%+v", parts)
	}
	if parts[0].Type != "text" {
		t.Fatalf("text part=%+v", parts[0])
	}
	for _, want := range []string{"看看附件", "【附件: notes.txt】", "hello attachment"} {
		if !strings.Contains(parts[0].Text, want) {
			t.Fatalf("text part missing %q: %q", want, parts[0].Text)
		}
	}
}
