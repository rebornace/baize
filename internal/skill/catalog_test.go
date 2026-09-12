package skill_test

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/skill"
)

func TestCatalogUserOverridesBuiltin(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	mustWriteSkill(t, filepath.Join(builtin, "demo"), "demo", "from-builtin", []string{"a"})
	mustWriteSkill(t, filepath.Join(user, "demo"), "demo", "from-user", []string{"b"})
	cat, err := skill.LoadCatalog([]string{builtin}, user)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cat.Get("demo")
	if !ok || p.Description != "from-user" || p.Source != "user" {
		t.Fatalf("%+v ok=%v", p, ok)
	}
}

func TestCatalogSkipsDirWithoutSkillMD(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	mustWriteSkill(t, filepath.Join(builtin, "good"), "good", "ok", []string{"a"})
	if err := os.MkdirAll(filepath.Join(builtin, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	cat, err := skill.LoadCatalog([]string{builtin}, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("empty"); ok {
		t.Fatal("empty dir should not be in catalog")
	}
	if _, ok := cat.Get("good"); !ok {
		t.Fatal("good skill should be loaded")
	}
}

func TestLoadCatalogRejectsInvalidSkillMD(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")

	t.Run("missing frontmatter", func(t *testing.T) {
		dir := filepath.Join(builtin, "bad-fm")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("no frontmatter\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := skill.LoadCatalog([]string{builtin}, user); err == nil {
			t.Fatal("expected error for missing frontmatter")
		}
	})

	t.Run("missing name", func(t *testing.T) {
		root2 := t.TempDir()
		builtin2 := filepath.Join(root2, "builtin")
		user2 := filepath.Join(root2, "user")
		dir := filepath.Join(builtin2, "bad-name")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		raw := "---\ndescription: x\n---\n\nbody\n"
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := skill.LoadCatalog([]string{builtin2}, user2); err == nil {
			t.Fatal("expected error for missing name")
		}
	})
}

func TestDeleteUser(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	mustWriteSkill(t, filepath.Join(builtin, "builtin-only"), "builtin-only", "x", []string{"a"})
	mustWriteSkill(t, filepath.Join(user, "user-skill"), "user-skill", "y", []string{"b"})

	cat, err := skill.LoadCatalog([]string{builtin}, user)
	if err != nil {
		t.Fatal(err)
	}

	if err := cat.DeleteUser("missing"); !errors.Is(err, skill.ErrNotFound) {
		t.Fatalf("missing: want ErrNotFound, got %v", err)
	}
	if err := cat.DeleteUser("builtin-only"); !errors.Is(err, skill.ErrBuiltin) {
		t.Fatalf("builtin: want ErrBuiltin, got %v", err)
	}
	if err := cat.DeleteUser("user-skill"); err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("user-skill"); ok {
		t.Fatal("user skill should be deleted")
	}
}

func TestInstallMDRejectsUnsafeName(t *testing.T) {
	root := t.TempDir()
	cat, err := skill.LoadCatalog([]string{filepath.Join(root, "builtin")}, filepath.Join(root, "user"))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("---\nname: ../evil\ndescription: x\n---\n\nbody\n")
	if _, err := cat.InstallMD("evil.md", raw); err == nil {
		t.Fatal("expected error for path traversal name")
	}
}

func TestInstallZipRejectsTraversal(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("../evil/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("---\nname: evil\ndescription: x\n---\n\nbody\n")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	cat, err := skill.LoadCatalog([]string{filepath.Join(root, "builtin")}, filepath.Join(root, "user"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.InstallZip(buf.Bytes()); err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestLoadCatalogReadsWorkflowYAML(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	dir := filepath.Join(builtin, "pipe")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: pipe\ntools:\n  - t\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"),
		[]byte("name: pipe\nsteps:\n  - id: a\n    tool: t\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cat, err := skill.LoadCatalog([]string{builtin}, user)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	p, ok := cat.Get("pipe")
	if !ok {
		t.Fatal("pkg missing")
	}
	if p.Workflow == nil || p.Workflow.Name != "pipe" || len(p.Workflow.Steps) != 1 {
		t.Fatalf("wf=%+v", p.Workflow)
	}
}

func TestLoadCatalogInvalidWorkflowFails(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	dir := filepath.Join(builtin, "bad")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: bad\n---\nb"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "workflow.yaml"), []byte("name:\nsteps: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.LoadCatalog([]string{builtin}, user); err == nil {
		t.Fatal("want invalid workflow to fail load")
	}
}

func TestLoadCatalogWithoutWorkflowYAML(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	mustWriteSkill(t, filepath.Join(builtin, "plain"), "plain", "no pipeline", []string{"a"})
	cat, err := skill.LoadCatalog([]string{builtin}, user)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cat.Get("plain")
	if !ok {
		t.Fatal("plain skill should be loaded")
	}
	if p.Workflow != nil {
		t.Fatalf("want nil Workflow, got %+v", p.Workflow)
	}
}

func TestLoadCatalogWorkflowReadErrorFails(t *testing.T) {
	root := t.TempDir()
	builtin := filepath.Join(root, "builtin")
	user := filepath.Join(root, "user")
	dir := filepath.Join(builtin, "broken")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\nname: broken\n---\nb"), 0o644); err != nil {
		t.Fatal(err)
	}
	// workflow.yaml 建成目录：ReadFile 对目录报非 NotExist 错误（Windows/Unix 均稳定）。
	if err := os.MkdirAll(filepath.Join(dir, "workflow.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := skill.LoadCatalog([]string{builtin}, user); err == nil {
		t.Fatal("want workflow.yaml read error to fail load")
	}
}

func TestLoadRepoTicketTriage(t *testing.T) {
	builtin := filepath.Join("..", "..", "examples", "skills")
	user := t.TempDir()
	cat, err := skill.LoadCatalog([]string{builtin}, user)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cat.Get("ticket-triage")
	if !ok {
		t.Fatal("ticket-triage not found in examples/skills")
	}
	if p.Description != "工单分诊与建单流程（mock-ticket）" {
		t.Fatalf("description=%q", p.Description)
	}
	wantTools := []string{"list_tickets", "get_ticket", "create_ticket", "update_ticket_status"}
	if len(p.Tools) != len(wantTools) {
		t.Fatalf("tools=%v want %v", p.Tools, wantTools)
	}
	for i, w := range wantTools {
		if p.Tools[i] != w {
			t.Fatalf("tools[%d]=%q want %q", i, p.Tools[i], w)
		}
	}
	if p.Source != skill.SourceBuiltin {
		t.Fatalf("source=%q want builtin", p.Source)
	}
	if !strings.Contains(p.Body, "list_tickets") {
		t.Fatalf("body should mention list_tickets: %q", p.Body)
	}
}

func TestLoadCatalogMultipleBuiltinDirs(t *testing.T) {
	core := filepath.Join("..", "..", "skills")
	demo := filepath.Join("..", "..", "examples", "skills")
	user := t.TempDir()
	cat, err := skill.LoadCatalog([]string{core, demo}, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("data-analytics"); !ok {
		t.Fatal("data-analytics not found")
	}
	if _, ok := cat.Get("ticket-triage"); !ok {
		t.Fatal("ticket-triage not found")
	}
	catMinimal, err := skill.LoadCatalog([]string{core}, user)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := catMinimal.Get("ticket-triage"); ok {
		t.Fatal("minimal scan should not include ticket-triage")
	}
}

func TestLoadRepoDataAnalytics(t *testing.T) {
	builtin := filepath.Join("..", "..", "skills")
	user := t.TempDir()
	cat, err := skill.LoadCatalog([]string{builtin}, user)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := cat.Get("data-analytics")
	if !ok {
		t.Fatal("data-analytics not found in skills/")
	}
	if p.Source != skill.SourceBuiltin {
		t.Fatalf("source=%q want builtin", p.Source)
	}
	if !containsString(p.Tools, "create_analysis_page") {
		t.Fatalf("tools=%v want create_analysis_page", p.Tools)
	}
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func mustWriteSkill(t *testing.T, dir, name, desc string, tools []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	b.WriteString("---\nname: " + name + "\ndescription: " + desc + "\ntools:\n")
	for _, x := range tools {
		b.WriteString("  - " + x + "\n")
	}
	b.WriteString("---\n\nbody\n")
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}
