package api

import (
	"context"
	"testing"
	"time"

	"github.com/rebornace/baize/internal/conversation"
)

type hydrateFakeMedia struct {
	data []byte
	mime string
}

func (f *hydrateFakeMedia) OpenMedia(context.Context, string, string) ([]byte, string, bool, error) {
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
