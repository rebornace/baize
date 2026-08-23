package skill

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const (
	SourceBuiltin = "builtin"
	SourceUser    = "user"
)

var (
	ErrNotFound = errors.New("skill not found")
	ErrBuiltin  = errors.New("cannot delete builtin skill")
)

type Catalog struct {
	mu          sync.RWMutex
	byID        map[string]Package
	builtinDirs []string
	userDir     string
}

func LoadCatalog(builtinDirs []string, user string) (*Catalog, error) {
	c := &Catalog{
		byID:        make(map[string]Package),
		builtinDirs: append([]string(nil), builtinDirs...),
		userDir:     user,
	}
	if err := c.Reload(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Catalog) Get(id string) (Package, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	p, ok := c.byID[id]
	return p, ok
}

func (c *Catalog) List() []Package {
	c.mu.RLock()
	defer c.mu.RUnlock()
	ids := make([]string, 0, len(c.byID))
	for id := range c.byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Package, 0, len(ids))
	for _, id := range ids {
		out = append(out, c.byID[id])
	}
	return out
}

func (c *Catalog) Reload() error {
	byID := make(map[string]Package)
	for _, dir := range c.builtinDirs {
		if err := scanDir(dir, SourceBuiltin, byID); err != nil {
			return err
		}
	}
	if err := scanDir(c.userDir, SourceUser, byID); err != nil {
		return err
	}
	c.mu.Lock()
	c.byID = byID
	c.mu.Unlock()
	return nil
}

func scanDir(dir, source string, byID map[string]Package) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		id := e.Name()
		skillPath := filepath.Join(dir, id, "SKILL.md")
		raw, err := os.ReadFile(skillPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		pkg, err := ParseSKILLMD(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", skillPath, err)
		}
		if pkg.Name != id {
			log.Printf("skill: warning: %s name=%q != id=%q", skillPath, pkg.Name, id)
		}
		pkg.ID = id
		pkg.Source = source
		pkg.Dir = filepath.Join(dir, id)
		byID[id] = pkg
	}
	return nil
}

func (c *Catalog) InstallMD(filename string, raw []byte) (Package, error) {
	_ = filename
	pkg, err := ParseSKILLMD(raw)
	if err != nil {
		return Package{}, err
	}
	id := pkg.Name
	if err := validateSkillID(id); err != nil {
		return Package{}, err
	}
	dir := filepath.Join(c.userDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Package{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), raw, 0o644); err != nil {
		return Package{}, err
	}
	if err := c.Reload(); err != nil {
		return Package{}, err
	}
	p, ok := c.Get(id)
	if !ok {
		return Package{}, fmt.Errorf("installed skill %q not found after reload", id)
	}
	return p, nil
}

func (c *Catalog) InstallZip(raw []byte) (Package, error) {
	zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return Package{}, err
	}
	for _, f := range zr.File {
		if !filepath.IsLocal(f.Name) {
			return Package{}, fmt.Errorf("invalid zip entry: %s", f.Name)
		}
	}

	tmpDir, err := os.MkdirTemp("", "skill-zip-*")
	if err != nil {
		return Package{}, err
	}
	defer os.RemoveAll(tmpDir)

	for _, f := range zr.File {
		dest := filepath.Join(tmpDir, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(dest, 0o755); err != nil {
				return Package{}, err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return Package{}, err
		}
		rc, err := f.Open()
		if err != nil {
			return Package{}, err
		}
		out, err := os.OpenFile(dest, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, f.Mode())
		if err != nil {
			rc.Close()
			return Package{}, err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := out.Close()
		rc.Close()
		if copyErr != nil {
			return Package{}, copyErr
		}
		if closeErr != nil {
			return Package{}, closeErr
		}
	}

	skillPath, err := findSkillMD(tmpDir)
	if err != nil {
		return Package{}, err
	}
	skillRaw, err := os.ReadFile(skillPath)
	if err != nil {
		return Package{}, err
	}
	return c.InstallMD(filepath.Base(skillPath), skillRaw)
}

func findSkillMD(root string) (string, error) {
	direct := filepath.Join(root, "SKILL.md")
	if _, err := os.Stat(direct); err == nil {
		return direct, nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", err
	}
	var subdirs []string
	for _, e := range entries {
		if e.IsDir() {
			subdirs = append(subdirs, e.Name())
		}
	}
	if len(subdirs) == 1 {
		p := filepath.Join(root, subdirs[0], "SKILL.md")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("SKILL.md not found")
}

func validateSkillID(id string) error {
	if !filepath.IsLocal(id) {
		return fmt.Errorf("invalid skill id: %q", id)
	}
	return nil
}

func (c *Catalog) DeleteUser(id string) error {
	p, ok := c.Get(id)
	if !ok {
		return fmt.Errorf("%w: %q", ErrNotFound, id)
	}
	if p.Source != SourceUser {
		return fmt.Errorf("%w: %q", ErrBuiltin, id)
	}
	if err := os.RemoveAll(filepath.Join(c.userDir, id)); err != nil {
		return err
	}
	return c.Reload()
}
