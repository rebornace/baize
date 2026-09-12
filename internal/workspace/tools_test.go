package workspace_test

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workspace"
)

func ctxWithConv(conv string) context.Context {
	return identity.WithConversationID(context.Background(), conv)
}

// memService builds a workspace.Service over an in-memory blob store, with
// vision enabled only when requested. It is separate from newSvc in
// service_test.go so the vision gate can be exercised end to end.
func memService(t *testing.T, vision bool) *workspace.Service {
	t.Helper()
	b, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	opts := []workspace.Option{}
	if vision {
		opts = append(opts, workspace.WithVision(func() bool { return true }))
	}
	return workspace.New(b, opts...)
}

func TestListAndWriteReadTools(t *testing.T) {
	svc := newSvc(t)
	tools := workspace.Tools(svc)
	reg := tool.NewRegistry()
	for _, tm := range tools {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	ctx := ctxWithConv("c1")

	// write_file
	c, isErr, err := reg.Invoke(ctx, "write_file", map[string]any{"path": "notes/a.md", "content": "# hi"})
	if err != nil || isErr {
		t.Fatalf("write_file err=%v isErr=%v c=%v", err, isErr, c)
	}
	// list_files at root -> notes dir
	c, isErr, _ = reg.Invoke(ctx, "list_files", map[string]any{})
	if isErr {
		t.Fatalf("list_files isErr: %v", c)
	}
	entries, _ := c["entries"].([]workspace.Entry)
	if len(entries) != 1 || entries[0].Name != "notes" || entries[0].Type != "dir" {
		t.Fatalf("want notes dir, got %v", c)
	}
	// read_file
	c, isErr, _ = reg.Invoke(ctx, "read_file", map[string]any{"path": "notes/a.md"})
	if isErr {
		t.Fatalf("read_file isErr: %v", c)
	}
	if c["content"] != "# hi" {
		t.Fatalf("content=%v", c["content"])
	}
	// delete_file
	c, isErr, _ = reg.Invoke(ctx, "delete_file", map[string]any{"path": "notes/a.md"})
	if isErr {
		t.Fatalf("delete_file isErr: %v", c)
	}
}

func TestToolsRequireConversation(t *testing.T) {
	svc := newSvc(t)
	reg := tool.NewRegistry()
	for _, tm := range workspace.Tools(svc) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	_, isErr, err := reg.Invoke(context.Background(), "list_files", map[string]any{})
	if err != nil || !isErr {
		t.Fatalf("want isError without conversation, got isErr=%v err=%v", isErr, err)
	}
}

func TestReadImageToolAttachesPart(t *testing.T) {
	// vision-enabled service
	svc := memService(t, true)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0, 0, 0, 0, 0}
	if _, err := svc.SaveUploadBytes(context.Background(), "c1", "s.png", png, "image/png"); err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	for _, tm := range workspace.Tools(svc) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	c, isErr, err := reg.Invoke(ctxWithConv("c1"), "read_image", map[string]any{"path": "uploads/s.png"})
	if err != nil || isErr {
		t.Fatalf("read_image err=%v isErr=%v c=%v", err, isErr, c)
	}
	_, results := tool.ExtractImageParts(c)
	if len(results) != 1 || results[0].Part.Type != "image" {
		t.Fatalf("want 1 image result attached, got %v", results)
	}
	if results[0].Path != "uploads/s.png" {
		t.Fatalf("image result path=%q", results[0].Path)
	}
}

func TestReadImageToolVisionGate(t *testing.T) {
	// default service: vision off
	svc := memService(t, false)
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	_, _ = svc.SaveUploadBytes(context.Background(), "c1", "s.png", png, "image/png")
	reg := tool.NewRegistry()
	for _, tm := range workspace.Tools(svc) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	_, isErr, err := reg.Invoke(ctxWithConv("c1"), "read_image", map[string]any{"path": "uploads/s.png"})
	if err != nil || !isErr {
		t.Fatalf("non-vision model must error, isErr=%v err=%v", isErr, err)
	}
}
