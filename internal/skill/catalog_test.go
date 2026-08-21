package skill_test

import (
	"archive/zip"
	"bytes"
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
	cat, err := skill.LoadCatalog(builtin, user)
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
	cat, err := skill.LoadCatalog(builtin, user)
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
		if _, err := skill.LoadCatalog(builtin, user); err == nil {
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
		if _, err := skill.LoadCatalog(builtin2, user2); err == nil {
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

	cat, err := skill.LoadCatalog(builtin, user)
	if err != nil {
		t.Fatal(err)
	}

	if err := cat.DeleteUser("missing"); err == nil {
		t.Fatal("expected error for unknown id")
	}
	if err := cat.DeleteUser("builtin-only"); err == nil {
		t.Fatal("expected error for builtin")
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
	cat, err := skill.LoadCatalog(filepath.Join(root, "builtin"), filepath.Join(root, "user"))
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
	cat, err := skill.LoadCatalog(filepath.Join(root, "builtin"), filepath.Join(root, "user"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cat.InstallZip(buf.Bytes()); err == nil {
		t.Fatal("expected error for path traversal")
	}
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
