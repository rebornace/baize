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

// SyncAll refreshes managed login skills for all openapi/http connectors.
func SyncAll(st store.Store, managedDir, userDir string) error {
	if st == nil {
		return nil
	}
	for _, c := range st.ListConnectors() {
		if !isLoginSkillConnectorType(c.Type) {
			continue
		}
		if err := SyncConnector(st, managedDir, userDir, c.ID); err != nil {
			return err
		}
	}
	return nil
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

	tools, skip := selectToolsForSync(st, connectorID)
	if skip {
		return nil
	}

	pkgName := skillID
	managedPkg := filepath.Join(managedDir, pkgName)
	userPkg := filepath.Join(userDir, pkgName)

	if conflictNonManaged(userPkg) || conflictNonManaged(managedPkg) {
		log.Printf("loginmanage: skip %s: non-managed package occupies skill id", skillID)
		return nil
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

func selectToolsForSync(st store.Store, connectorID string) (tools []string, skip bool) {
	c, err := st.GetConnector(connectorID)
	if err != nil {
		// Connector gone: treat as empty tools so managed package can be removed.
		return nil, false
	}
	if !isLoginSkillConnectorType(c.Type) {
		return nil, true
	}
	return SelectTools(st, connectorID), false
}

func isLoginSkillConnectorType(typ string) bool {
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "openapi", "http":
		return true
	default:
		return false
	}
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
