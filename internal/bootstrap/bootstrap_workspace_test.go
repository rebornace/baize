package bootstrap

import (
	"context"
	"testing"

	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/tool"
	"github.com/rebornace/baize/internal/workspace"
)

// TestWorkspaceToolsRegistered is a lightweight assembly smoke test: it wires
// a workspace.Service over an in-memory blob store into a tool.Registry the
// same way newAPIServer does and asserts all 5 built-in file tools are
// registered under their expected names.
func TestWorkspaceToolsRegistered(t *testing.T) {
	blobs, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	reg := tool.NewRegistry()
	ws := workspace.New(blobs, workspace.WithVision(func() bool { return true }))
	for _, tm := range workspace.Tools(ws) {
		reg.RegisterSpecApproved(tm.Spec, tm.Invoker, false)
	}
	want := map[string]bool{
		"list_files":  false,
		"read_file":   false,
		"write_file":  false,
		"delete_file": false,
		"read_image":  false,
	}
	for _, info := range reg.List() {
		if _, ok := want[info.Name]; ok {
			want[info.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Fatalf("workspace tool %q not registered", name)
		}
	}
}
