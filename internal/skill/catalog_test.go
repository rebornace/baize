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
