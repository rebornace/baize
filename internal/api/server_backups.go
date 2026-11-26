package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const backupFilePrefix = "baize-backup-"

type backupInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) sqlitePath() string {
	if s.Config != nil && strings.TrimSpace(s.Config.Store.SQLitePath) != "" {
		return s.Config.Store.SQLitePath
	}
	return "./data/baize.db"
}

func (s *Server) backupDir() string {
	return filepath.Join(filepath.Dir(s.sqlitePath()), "backups")
}

func (s *Server) listBackups() ([]backupInfo, error) {
	dir := s.backupDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return []backupInfo{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := make([]backupInfo, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		out = append(out, backupInfo{
			Name: entry.Name(), Size: info.Size(), CreatedAt: info.ModTime().UTC(),
		})
	}
	return out, nil
}

func (s *Server) handleListBackups(w http.ResponseWriter, r *http.Request) {
	items, err := s.listBackups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items, "dir": s.backupDir()})
}

func (s *Server) handleCreateBackup(w http.ResponseWriter, r *http.Request) {
	dir := s.backupDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	name := backupFilePrefix + time.Now().UTC().Format("20060102-150405") + ".db"
	path := filepath.Join(dir, name)
	if err := s.Store.BackupTo(path); err != nil {
		if errors.Is(err, store.ErrBackupUnsupported) {
			writeError(w, http.StatusNotImplemented, "backup_unsupported",
				"in-app backups currently support SQLite; use pg_dump / PITR scripts for Postgres")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, backupInfo{
		Name: name, Size: info.Size(), CreatedAt: info.ModTime().UTC(),
	})
}

func (s *Server) handleDownloadBackup(w http.ResponseWriter, r *http.Request) {
	name := filepath.Base(strings.TrimSpace(r.PathValue("name")))
	if name == "." || name == "" || !strings.HasPrefix(name, backupFilePrefix) ||
		!strings.HasSuffix(name, ".db") {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid backup name")
		return
	}
	path := filepath.Join(s.backupDir(), name)
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "not_found", "backup not found")
		return
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	http.ServeFile(w, r, path)
}

// handleStageRestore accepts a SQLite backup and stages it for the next start.
// The running database is never modified in place.
func (s *Server) handleStageRestore(w http.ResponseWriter, r *http.Request) {
	if s.Config != nil && s.Config.Store.Driver != "" && s.Config.Store.Driver != "sqlite" {
		writeError(w, http.StatusNotImplemented, "restore_unsupported",
			"in-app restore currently supports SQLite; restore Postgres backups with pg_restore")
		return
	}
	if err := r.ParseMultipartForm(64 << 20); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid upload")
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "file is required")
		return
	}
	defer file.Close()
	dbPath := s.sqlitePath()
	pending := dbPath + ".restore"
	tmp := pending + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	if _, err := io.Copy(out, io.LimitReader(file, 64<<20)); err != nil {
		_ = out.Close()
		_ = os.Remove(tmp)
		writeError(w, http.StatusBadRequest, "invalid_request", "upload failed")
		return
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmp)
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	header := make([]byte, 16)
	f, err := os.Open(tmp)
	if err == nil {
		_, _ = f.Read(header)
		_ = f.Close()
	}
	if string(header) != "SQLite format 3\x00" {
		_ = os.Remove(tmp)
		writeError(w, http.StatusBadRequest, "invalid_request", "not a SQLite database")
		return
	}
	if err := os.Rename(tmp, pending); err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", err.Error())
		return
	}
	raw, _ := json.Marshal(map[string]any{"staged_at": time.Now().UTC(), "path": pending})
	_ = os.WriteFile(dbPath+".restore-pending", raw, 0o600)
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "staged",
		"message": "restart Baize to apply the restored database",
	})
}
