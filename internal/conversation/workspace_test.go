package conversation_test

import (
	"testing"

	"github.com/rebornace/baize/internal/conversation"
)

func TestMemoryWorkspacesDefaultAndCreate(t *testing.T) {
	s := conversation.NewMemoryStore()
	list, err := s.ListWorkspaces()
	if err != nil || len(list) != 1 || list[0].ID != conversation.DefaultWorkspaceID {
		t.Fatalf("default list=%+v err=%v", list, err)
	}
	created, err := s.CreateWorkspace("实验室")
	if err != nil || created.ID == conversation.DefaultWorkspaceID {
		t.Fatalf("create=%+v err=%v", created, err)
	}
	list, err = s.ListWorkspaces()
	if err != nil || len(list) != 2 {
		t.Fatalf("after create list=%+v err=%v", list, err)
	}
	if _, err := s.CreateWorkspace("  "); err == nil {
		t.Fatal("empty name should fail")
	}
}

func TestEnsureMetaAssignsDefaultWorkspaceForUI(t *testing.T) {
	s := conversation.NewMemoryStore()
	if err := s.EnsureMeta(conversation.Meta{ID: "conv_a", OwnerID: "local-dev", Source: "ui"}); err != nil {
		t.Fatal(err)
	}
	m, err := s.GetMeta("conv_a")
	if err != nil || m.WorkspaceID != conversation.DefaultWorkspaceID {
		t.Fatalf("meta=%+v err=%v", m, err)
	}
	if err := s.EnsureMeta(conversation.Meta{ID: "weixin:a:p", OwnerID: "alice", Source: "weixin"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.GetMeta("weixin:a:p")
	if err != nil || ch.WorkspaceID != "" {
		t.Fatalf("channel meta should not join a web workspace: %+v", ch)
	}
}

func TestListMetaFiltersWorkspace(t *testing.T) {
	s := conversation.NewMemoryStore()
	_ = s.EnsureMeta(conversation.Meta{ID: "c1", OwnerID: "a", Source: "ui", WorkspaceID: conversation.DefaultWorkspaceID})
	_ = s.EnsureMeta(conversation.Meta{ID: "c2", OwnerID: "a", Source: "ui", WorkspaceID: "ws_lab"})
	got, err := s.ListMeta(conversation.MetaFilter{WorkspaceID: "ws_lab"})
	if err != nil || len(got) != 1 || got[0].ID != "c2" {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}
