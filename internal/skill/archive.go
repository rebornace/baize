package skill

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/blob"
)

const SourceArchived = "archived"

// ArchiveUser moves skills/user/<id>/ to skills/archived/<id>/ and reloads.
// Does not delete objects. Builtin/managed skills cannot be archived this way.
func (c *Catalog) ArchiveUser(id string) error {
	if c == nil || c.Blobs == nil {
		return fmt.Errorf("catalog blobs not configured")
	}
	if err := validateSkillID(id); err != nil {
		return err
	}
	pkg, ok := c.Get(id)
	if !ok {
		return ErrNotFound
	}
	if pkg.Source != SourceUser {
		return fmt.Errorf("only user skills can be archived (got %s)", pkg.Source)
	}
	ctx := context.Background()
	from := blob.PrefixSkillsUser + id + "/"
	to := blob.PrefixSkillsArchived + id + "/"
	if err := blob.MovePrefix(ctx, c.Blobs, from, to); err != nil {
		return err
	}
	return c.Reload()
}

// UnarchiveUser moves skills/archived/<id>/ back to skills/user/<id>/ and reloads.
func (c *Catalog) UnarchiveUser(id string) error {
	if c == nil || c.Blobs == nil {
		return fmt.Errorf("catalog blobs not configured")
	}
	if err := validateSkillID(id); err != nil {
		return err
	}
	if _, ok := c.Get(id); ok {
		return fmt.Errorf("skill %q already active in catalog", id)
	}
	ctx := context.Background()
	from := blob.PrefixSkillsArchived + id + "/"
	to := blob.PrefixSkillsUser + id + "/"
	if err := blob.MovePrefix(ctx, c.Blobs, from, to); err != nil {
		return err
	}
	return c.Reload()
}

// ListArchivedIDs returns skill ids that currently live under skills/archived/.
func (c *Catalog) ListArchivedIDs() ([]string, error) {
	if c == nil || c.Blobs == nil {
		return nil, nil
	}
	entries, err := c.Blobs.List(context.Background(), blob.PrefixSkillsArchived)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	var out []string
	for _, e := range entries {
		id, ok := skillIDFromBlobKey(blob.PrefixSkillsArchived, e.Key)
		if !ok {
			rest, ok := strings.CutPrefix(e.Key, blob.PrefixSkillsArchived)
			if !ok {
				continue
			}
			id, _, ok = strings.Cut(rest, "/")
			if !ok || id == "" {
				continue
			}
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out, nil
}

// SeedUsageInstalled ensures every currently loaded user skill has an
// installed_at row (without bumping activate counts).
func SeedUsageInstalled(cat *Catalog, usage UsageStore, at time.Time) {
	if cat == nil || usage == nil {
		return
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	for _, pkg := range cat.List() {
		if pkg.Source != SourceUser {
			continue
		}
		_ = usage.EnsureInstalled(pkg.ID, at)
	}
}

// CurateArchives moves idle user skills to archived/ when older than afterDays.
// afterDays <= 0 disables curation. Returns archived skill ids.
func CurateArchives(cat *Catalog, usage UsageStore, afterDays int, now time.Time) []string {
	if cat == nil || usage == nil || afterDays <= 0 {
		return nil
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cutoff := now.Add(-time.Duration(afterDays) * 24 * time.Hour)
	var archived []string
	for _, pkg := range cat.List() {
		if pkg.Source != SourceUser {
			continue
		}
		u, ok := usage.Get(pkg.ID)
		if !ok {
			_ = usage.EnsureInstalled(pkg.ID, now)
			continue
		}
		ref := u.LastActivatedAt
		if ref.IsZero() {
			ref = u.InstalledAt
		}
		if ref.IsZero() || !ref.Before(cutoff) {
			continue
		}
		if err := cat.ArchiveUser(pkg.ID); err != nil {
			log.Printf("skill curator: archive %s: %v", pkg.ID, err)
			continue
		}
		log.Printf("skill curator: archived user skill %s (idle since %s)", pkg.ID, ref.Format(time.RFC3339))
		archived = append(archived, pkg.ID)
	}
	return archived
}
