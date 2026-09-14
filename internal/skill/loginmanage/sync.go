package loginmanage

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/rebornace/baize/internal/store"
)

// SyncAll refreshes managed login skills for all connectors, then removes orphan
// connector_login packages (connector missing or not openapi/http).
func SyncAll(st store.Store, managedDir, userDir string) error {
	if st == nil {
		return nil
	}
	for _, c := range st.ListConnectors() {
		if err := SyncConnector(st, managedDir, userDir, c.ID); err != nil {
			return err
		}
	}
	return cleanupOrphanManagedLoginSkills(st, managedDir)
}

// SyncConnector writes, updates, or deletes the managed login skill for one connector.
func SyncConnector(st store.Store, managedDir, userDir, connectorID string) error {
	if st == nil {
		return nil
	}
	skillID := SkillID(connectorID)
	if skillID == "" {
		log.Printf("loginmanage: skip connector %q: empty normalized id", connectorID)
		return nil
	}

	tools := selectToolsForSync(st, connectorID)

	pkgName := skillID
	managedPkg := filepath.Join(managedDir, pkgName)
	userPkg := filepath.Join(userDir, pkgName)

	// Non-managed conflict: do not overwrite user (or non-managed) package.
	// Delete residual managed connector_login so Catalog managed-over-user
	// load order cannot shadow the user fork.
	if conflictNonManaged(userPkg) || conflictNonManaged(managedPkg) {
		log.Printf("loginmanage: skip %s: non-managed package occupies skill id", skillID)
		return deleteManagedPackage(managedPkg)
	}

	if len(tools) == 0 {
		return deleteManagedPackage(managedPkg)
	}

	content, err := RenderSKILLMD(connectorID, tools)
	if err != nil {
		return err
	}
	return writeManagedPackage(managedPkg, content)
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

func cleanupOrphanManagedLoginSkills(st store.Store, managedDir string) error {
	entries, err := os.ReadDir(managedDir)
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
		pkgDir := filepath.Join(managedDir, e.Name())
		raw, err := os.ReadFile(filepath.Join(pkgDir, "SKILL.md"))
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
				if SkillID(connectorID) == e.Name() {
					keep = true
				}
			}
		}
		if keep {
			continue
		}
		if err := deleteManagedPackage(pkgDir); err != nil {
			return err
		}
	}
	return nil
}

func conflictNonManaged(pkgDir string) bool {
	raw, err := os.ReadFile(filepath.Join(pkgDir, "SKILL.md"))
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

func writeManagedPackage(pkgDir, content string) error {
	if err := os.RemoveAll(pkgDir); err != nil {
		return fmt.Errorf("remove managed package: %w", err)
	}
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return fmt.Errorf("mkdir managed package: %w", err)
	}
	path := filepath.Join(pkgDir, "SKILL.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}
	return nil
}

func deleteManagedPackage(pkgDir string) error {
	raw, err := os.ReadFile(filepath.Join(pkgDir, "SKILL.md"))
	if err != nil {
		if os.IsNotExist(err) {
			_ = os.RemoveAll(pkgDir)
			return nil
		}
		return err
	}
	if !isManagedConnectorLogin(raw) {
		return nil
	}
	if err := os.RemoveAll(pkgDir); err != nil {
		return fmt.Errorf("delete managed package: %w", err)
	}
	return nil
}
