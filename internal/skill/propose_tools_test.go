package skill_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/rebornace/baize/internal/controlplane"
	"github.com/rebornace/baize/internal/skill"
	"github.com/rebornace/baize/internal/tool"
)

func proposeTestCatalog(t *testing.T) *skill.Catalog {
	t.Helper()
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	if err := os.MkdirAll(builtin, 0o755); err != nil {
		t.Fatal(err)
	}
	cat, err := skill.LoadCatalog([]string{builtin}, filepath.Join(root, "user"), testMemoryBlobs(t))
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func proposeDemoArgs() map[string]any {
	return map[string]any{
		"name":        "demo",
		"description": "Demo skill",
		"body":        "Do the thing",
	}
}

func TestProposeInvokerInstallsAfterCall(t *testing.T) {
	cat := proposeTestCatalog(t)
	reg := tool.NewRegistry()
	tm := skill.ProposeToolMeta(cat)
	reg.RegisterSpecApproved(tm.Spec, tm.Invoker, true)
	_ = reg.RegisterArgsPrecheck(skill.ProposeName, skill.ValidateProposeArgs)

	out, isErr, err := reg.Invoke(context.Background(), skill.ProposeName, proposeDemoArgs())
	if err != nil || isErr {
		t.Fatalf("invoke err=%v isErr=%v out=%v", err, isErr, out)
	}
	if out["ok"] != true || out["id"] != "demo" {
		t.Fatalf("out=%v", out)
	}
	pkg, ok := cat.Get("demo")
	if !ok {
		t.Fatal("Get demo failed")
	}
	if pkg.Description != "Demo skill" {
		t.Fatalf("description=%q", pkg.Description)
	}
}

func TestProposeInvokerOverwritesUserSkill(t *testing.T) {
	cat := proposeTestCatalog(t)
	old := []byte("---\nname: demo\ndescription: old\ntools: []\n---\n\nold body\n")
	if _, err := cat.InstallMD("demo.md", old); err != nil {
		t.Fatal(err)
	}
	tm := skill.ProposeToolMeta(cat)
	out, isErr, err := tm.Invoker(context.Background(), proposeDemoArgs())
	if err != nil || isErr {
		t.Fatalf("invoke err=%v isErr=%v out=%v", err, isErr, out)
	}
	if out["id"] != "demo" {
		t.Fatalf("out=%v", out)
	}
	pkg, ok := cat.Get("demo")
	if !ok || pkg.Description != "Demo skill" {
		t.Fatalf("pkg=%+v ok=%v", pkg, ok)
	}
}

func TestProposeInvokerOperatorRoleDenied(t *testing.T) {
	cat := proposeTestCatalog(t)
	tm := skill.ProposeToolMeta(cat)
	ctx := controlplane.WithRole(context.Background(), controlplane.RoleOperator)
	_, isErr, err := tm.Invoker(ctx, proposeDemoArgs())
	if err != nil {
		t.Fatal(err)
	}
	if !isErr {
		t.Fatal("want permission error for operator role")
	}
	if _, ok := cat.Get("demo"); ok {
		t.Fatal("catalog must not change")
	}
}
