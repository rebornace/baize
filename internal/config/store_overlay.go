package config

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LocalOverlayPath returns the local override path for a base config file
// (e.g. configs/minimal.yaml -> configs/minimal.local.yaml).
func LocalOverlayPath(basePath string) string {
	ext := filepath.Ext(basePath)
	base := strings.TrimSuffix(basePath, ext)
	return base + ".local" + ext
}

// OverlayWritePath returns the file path used for persisting settings overlays.
func OverlayWritePath(baseConfigPath string) string {
	base := filepath.Base(baseConfigPath)
	if strings.Contains(base, ".local.") {
		return baseConfigPath
	}
	return LocalOverlayPath(baseConfigPath)
}

// StoreOverlay is the persisted store section from settings UI.
type StoreOverlay struct {
	Driver     string `yaml:"driver"`
	SQLitePath string `yaml:"sqlite_path,omitempty"`
	DSN        string `yaml:"dsn,omitempty"`
}

// WriteStoreOverlay atomically writes the store section into the local overlay YAML.
func WriteStoreOverlay(baseConfigPath string, overlay StoreOverlay) error {
	if strings.TrimSpace(baseConfigPath) == "" {
		return fmt.Errorf("config path is required")
	}
	overlayPath := OverlayWritePath(baseConfigPath)
	dir := filepath.Dir(overlayPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	doc := map[string]any{}
	if b, err := os.ReadFile(overlayPath); err == nil {
		_ = yaml.Unmarshal(b, &doc)
	}
	if doc == nil {
		doc = map[string]any{}
	}

	storeSec := map[string]any{"driver": overlay.Driver}
	if overlay.Driver == "sqlite" && strings.TrimSpace(overlay.SQLitePath) != "" {
		storeSec["sqlite_path"] = overlay.SQLitePath
	}
	if overlay.Driver == "postgres" && strings.TrimSpace(overlay.DSN) != "" {
		storeSec["dsn"] = overlay.DSN
	}
	doc["store"] = storeSec

	out, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	tmp := overlayPath + ".tmp"
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, overlayPath)
}

// dsnKeywordPassword matches the libpq keyword/value DSN credential, e.g.
// `password=secret`, `PASSWORD='s e'`, `password="p""w"`. It also matches a
// password= entry inside a URL query string (?/&). The leading boundary and
// the key's original casing are preserved on rewrite.
var dsnKeywordPassword = regexp.MustCompile(`(?i)(^|[\s?&])(password)=(?:'[^']*'|"(?:[^"]|"")*"|[^\s&]+)`)

// RedactDSN masks credentials in a DSN for display. It handles both the URL
// form (postgres://user:secret@host/db) and the libpq keyword/value form
// (host=... password=...). Unrecognized strings are returned unchanged.
func RedactDSN(dsn string) string {
	if dsn == "" {
		return ""
	}
	out := dsn
	// URL form userinfo: postgres://user:secret@host/db -> postgres://user:***@host/db
	if i := strings.Index(out, "://"); i >= 0 {
		rest := out[i+3:]
		if at := strings.Index(rest, "@"); at > 0 {
			userInfo := rest[:at]
			if colon := strings.Index(userInfo, ":"); colon >= 0 {
				out = out[:i+3] + userInfo[:colon+1] + "***" + rest[at:]
			}
		}
	}
	// Keyword/value form (also covers a password= in a URL query string):
	// password=secret -> password=***
	return dsnKeywordPassword.ReplaceAllString(out, "${1}${2}=***")
}
