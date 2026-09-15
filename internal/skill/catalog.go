package skill

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/workflow"
)

const (
	SourceBuiltin = "builtin"
	SourceUser    = "user"
	SourceManaged = "managed"
)

var (
	ErrNotFound = errors.New("skill not found")
	ErrBuiltin  = errors.New("cannot delete builtin skill")
	ErrManaged  = errors.New("cannot delete managed skill")
)

type Catalog struct {
	mu          sync.RWMutex
	byID        map[string]Package
	builtinDirs []string
	userDir     string // retained for API paths; user packages live in Blobs
	Blobs       blob.Store
}

func LoadCatalog(builtinDirs []string, userDir string, blobs blob.Store) (*Catalog, error) {
	c := &Catalog{
		byID:        make(map[string]Package),
		builtinDirs: append([]string(nil), builtinDirs...),
		userDir:     userDir,
		Blobs:       blobs,
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

func (c *Catalog) UserDir() string {
	if c == nil {
		return ""
	}
	return c.userDir
}

func (c *Catalog) Reload() error {
	byID := make(map[string]Package)
	for _, dir := range c.builtinDirs {
		if err := scanDir(dir, SourceBuiltin, byID); err != nil {
			return err
		}
	}
	if err := c.loadUserFromBlobs(byID); err != nil {
		return err
	}
	if err := c.loadManagedFromBlobs(byID); err != nil {
		return err
	}
	c.mu.Lock()
	c.byID = byID
	c.mu.Unlock()
	return nil
}

func (c *Catalog) loadUserFromBlobs(byID map[string]Package) error {
	return c.loadSkillsFromBlobs(blob.PrefixSkillsUser, SourceUser, byID)
}

func (c *Catalog) loadManagedFromBlobs(byID map[string]Package) error {
	return c.loadSkillsFromBlobs(blob.PrefixSkillsManaged, SourceManaged, byID)
}

func (c *Catalog) loadSkillsFromBlobs(prefix, source string, byID map[string]Package) error {
	if c.Blobs == nil {
		return nil
	}
	ctx := context.Background()
	entries, err := c.Blobs.List(ctx, prefix)
	if err != nil {
		return err
	}
	for _, e := range entries {
		id, ok := skillIDFromBlobKey(prefix, e.Key)
		if !ok {
			continue
		}
		raw, err := c.Blobs.Get(ctx, e.Key)
		if err != nil {
			return fmt.Errorf("skill blob %s: %w", e.Key, err)
		}
		pkg, err := ParseSKILLMD(raw)
		if err != nil {
			return fmt.Errorf("%s: %w", e.Key, err)
		}
		if pkg.Name != id {
			log.Printf("skill: warning: %s name=%q != id=%q", e.Key, pkg.Name, id)
		}
		pkg.ID = id
		pkg.Source = source
		wfKey := blob.SkillObjectKey(source, id, "workflow.yaml")
		wfRaw, wfErr := c.Blobs.Get(ctx, wfKey)
		if wfErr == nil {
			wf, perr := workflow.Parse(wfRaw)
			if perr != nil {
				return fmt.Errorf("%s: %w", wfKey, perr)
			}
			if wf.Name != pkg.ID {
				log.Printf("skill: warning: %s workflow name=%q != id=%q", wfKey, wf.Name, pkg.ID)
			}
			pkg.Workflow = wf
		} else if !errors.Is(wfErr, blob.ErrNotFound) {
			return fmt.Errorf("%s: %w", wfKey, wfErr)
		}
		byID[id] = pkg
	}
	return nil
}

// skillIDFromBlobKey extracts id from skills/<source>/<id>/SKILL.md.
func skillIDFromBlobKey(prefix, key string) (string, bool) {
	rest, ok := strings.CutPrefix(key, prefix)
	if !ok {
		return "", false
	}
	id, suffix, ok := strings.Cut(rest, "/")
	if !ok || id == "" || suffix != "SKILL.md" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
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
		wfPath := filepath.Join(pkg.Dir, "workflow.yaml")
		wfRaw, wfErr := os.ReadFile(wfPath)
		if wfErr == nil {
			wf, perr := workflow.Parse(wfRaw)
			if perr != nil {
				return fmt.Errorf("%s: %w", wfPath, perr)
			}
			if wf.Name != pkg.ID {
				log.Printf("skill: warning: %s workflow name=%q != id=%q", pkg.Dir, wf.Name, pkg.ID)
			}
			pkg.Workflow = wf
		} else if !os.IsNotExist(wfErr) {
			return fmt.Errorf("%s: %w", wfPath, wfErr)
		}
		byID[id] = pkg
	}
	return nil
}

func (c *Catalog) InstallMD(filename string, raw []byte) (Package, error) {
	_ = filename
	if c.Blobs == nil {
		return Package{}, fmt.Errorf("blob store not configured")
	}
	pkg, err := ParseSKILLMD(raw)
	if err != nil {
		return Package{}, err
	}
	id := pkg.Name
	if err := validateSkillID(id); err != nil {
		return Package{}, err
	}
	ctx := context.Background()
	prefix := blob.PrefixSkillsUser + id + "/"
	if err := blob.DeletePrefix(ctx, c.Blobs, prefix); err != nil {
		return Package{}, err
	}
	key := blob.SkillObjectKey(SourceUser, id, "SKILL.md")
	if err := c.Blobs.Put(ctx, key, raw, "text/markdown"); err != nil {
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
	if c.Blobs == nil {
		return Package{}, fmt.Errorf("blob store not configured")
	}
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
	pkg, err := ParseSKILLMD(skillRaw)
	if err != nil {
		return Package{}, err
	}
	id := pkg.Name
	if err := validateSkillID(id); err != nil {
		return Package{}, err
	}

	pkgRoot := filepath.Dir(skillPath)
	ctx := context.Background()
	prefix := blob.PrefixSkillsUser + id + "/"
	if err := blob.DeletePrefix(ctx, c.Blobs, prefix); err != nil {
		return Package{}, err
	}
	err = filepath.WalkDir(pkgRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(pkgRoot, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		key := blob.SkillObjectKey(SourceUser, id, filepath.ToSlash(rel))
		ct := "application/octet-stream"
		if strings.EqualFold(filepath.Base(rel), "SKILL.md") {
			ct = "text/markdown"
		}
		return c.Blobs.Put(ctx, key, data, ct)
	})
	if err != nil {
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
	if p.Source == SourceManaged {
		return fmt.Errorf("%w: %q", ErrManaged, id)
	}
	if p.Source != SourceUser {
		return fmt.Errorf("%w: %q", ErrBuiltin, id)
	}
	if c.Blobs == nil {
		return fmt.Errorf("blob store not configured")
	}
	if err := blob.DeletePrefix(context.Background(), c.Blobs, blob.PrefixSkillsUser+id+"/"); err != nil {
		return err
	}
	return c.Reload()
}
