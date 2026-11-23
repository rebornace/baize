package loginmanage

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/store"
)

// SyncAll refreshes managed login skills for all connectors, then removes orphan
// connector_login packages (connector missing or not openapi/http).
func SyncAll(st store.Store, blobs blob.Store) error {
	if st == nil {
		return nil
	}
	for _, c := range st.ListConnectors() {
		if err := SyncConnector(st, blobs, c.ID); err != nil {
			return err
		}
	}
	return cleanupOrphanManagedLoginSkills(st, blobs)
}

// SyncConnector writes, updates, or deletes the managed login skill for one connector.
func SyncConnector(st store.Store, blobs blob.Store, connectorID string) error {
	if st == nil {
		return nil
	}
	if blobs == nil {
		return fmt.Errorf("blob store not configured")
	}
	skillID := SkillID(connectorID)
	if skillID == "" {
		log.Printf("loginmanage: skip connector %q: empty normalized id", connectorID)
		return nil
	}

	tools := selectToolsForSync(st, connectorID)
	ctx := context.Background()

	// Non-managed conflict: do not overwrite user (or non-managed) package.
	// Delete residual managed connector_login so Catalog managed-over-user
	// load order cannot shadow the user fork.
	userKey := blob.SkillObjectKey("user", skillID, "SKILL.md")
	managedKey := blob.SkillObjectKey("managed", skillID, "SKILL.md")
	if conflictNonManaged(ctx, blobs, userKey) || conflictNonManaged(ctx, blobs, managedKey) {
		log.Printf("loginmanage: skip %s: non-managed package occupies skill id", skillID)
		return deleteManagedPackage(ctx, blobs, skillID)
	}

	if len(tools) == 0 {
		return deleteManagedPackage(ctx, blobs, skillID)
	}

	content, err := RenderSKILLMD(connectorID, tools)
	if err != nil {
		return err
	}
	return writeManagedPackage(ctx, blobs, skillID, content)
}

// selectToolsForSync returns login tools for openapi/http connectors.
// Missing connectors and non-target types yield an empty slice so callers delete
// any existing managed connector_login package.
func selectToolsForSync(st store.Store, connectorID string) []string {
	c, err := st.GetConnector(connectorID)
	if err != nil {
		return nil
	}
	if !isLoginSkillConnectorType(c.Type) {
		return nil
	}
	return SelectTools(st, connectorID)
}

func isLoginSkillConnectorType(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "openapi", "http":
		return true
	default:
		return false
	}
}

func cleanupOrphanManagedLoginSkills(st store.Store, blobs blob.Store) error {
	if blobs == nil {
		return nil
	}
	ctx := context.Background()
	entries, err := blobs.List(ctx, blob.PrefixSkillsManaged)
	if err != nil {
		return err
	}
	for _, e := range entries {
		id, ok := managedSkillIDFromKey(e.Key)
		if !ok {
			continue
		}
		raw, err := blobs.Get(ctx, e.Key)
		if err != nil {
			continue
		}
		fm, ok := parseManagedFrontmatter(raw)
		if !ok || !fm.Managed || fm.ManagedKind != managedKindConnectorLogin {
			continue
		}
		connectorID := strings.TrimSpace(fm.ManagedConnectorID)
		keep := false
		if connectorID != "" {
			if c, err := st.GetConnector(connectorID); err == nil && isLoginSkillConnectorType(c.Type) {
				if SkillID(connectorID) == id {
					keep = true
				}
			}
		}
		if keep {
			continue
		}
		if err := deleteManagedPackage(ctx, blobs, id); err != nil {
			return err
		}
	}
	return nil
}

func managedSkillIDFromKey(key string) (string, bool) {
	rest, ok := strings.CutPrefix(key, blob.PrefixSkillsManaged)
	if !ok {
		return "", false
	}
	id, suffix, ok := strings.Cut(rest, "/")
	if !ok || id == "" || suffix != "SKILL.md" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

func conflictNonManaged(ctx context.Context, blobs blob.Store, key string) bool {
	raw, err := blobs.Get(ctx, key)
	if err != nil {
		return false
	}
	return !isManagedConnectorLogin(raw)
}

func isManagedConnectorLogin(raw []byte) bool {
	fm, ok := parseManagedFrontmatter(raw)
	if !ok {
		return false
	}
	return fm.Managed && fm.ManagedKind == managedKindConnectorLogin
}

func parseManagedFrontmatter(raw []byte) (skillFrontmatter, bool) {
	const delim = "---"
	s := string(raw)
	if !strings.HasPrefix(strings.TrimSpace(s), delim) {
		return skillFrontmatter{}, false
	}
	rest := strings.TrimSpace(s)
	rest = strings.TrimPrefix(rest, delim)
	end := strings.Index(rest, "\n"+delim)
	if end < 0 {
		return skillFrontmatter{}, false
	}
	var fm skillFrontmatter
	if err := yaml.Unmarshal([]byte(rest[:end]), &fm); err != nil {
		return skillFrontmatter{}, false
	}
	return fm, true
}

func writeManagedPackage(ctx context.Context, blobs blob.Store, id, content string) error {
	prefix := blob.PrefixSkillsManaged + id + "/"
	if err := blob.DeletePrefix(ctx, blobs, prefix); err != nil {
		return fmt.Errorf("remove managed package: %w", err)
	}
	key := blob.SkillObjectKey("managed", id, "SKILL.md")
	if err := blobs.Put(ctx, key, []byte(content), "text/markdown"); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}
	return nil
}

func deleteManagedPackage(ctx context.Context, blobs blob.Store, id string) error {
	key := blob.SkillObjectKey("managed", id, "SKILL.md")
	raw, err := blobs.Get(ctx, key)
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return nil
		}
		return err
	}
	if !isManagedConnectorLogin(raw) {
		return nil
	}
	if err := blob.DeletePrefix(ctx, blobs, blob.PrefixSkillsManaged+id+"/"); err != nil {
		return fmt.Errorf("delete managed package: %w", err)
	}
	return nil
}
