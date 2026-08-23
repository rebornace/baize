package artifact

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rebornace/baize/internal/store"
)

const sqliteArtifactsSchema = `
CREATE TABLE IF NOT EXISTS artifacts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
`

// FileStore persists HTML artifacts on disk with metadata in SQLite.
type FileStore struct {
	dir string
	db  *sql.DB
}

var _ Store = (*FileStore)(nil)

// NewFileStore creates a FileStore writing HTML files under dir and recording
// metadata in st's SQLite database.
func NewFileStore(dir string, st *store.SQLite) (*FileStore, error) {
	if st == nil {
		return nil, fmt.Errorf("store is nil")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create artifacts dir: %w", err)
	}
	db := st.DB()
	if _, err := db.Exec(sqliteArtifactsSchema); err != nil {
		return nil, fmt.Errorf("migrate artifacts schema: %w", err)
	}
	return &FileStore{dir: dir, db: db}, nil
}

// PutHTML writes html to disk and records its association with runID.
func (f *FileStore) PutHTML(runID string, html string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := "art_" + hex.EncodeToString(b)
	now := time.Now().Unix()

	path := filepath.Join(f.dir, id+".html")
	if err := os.WriteFile(path, []byte(html), 0o644); err != nil {
		return "", err
	}

	_, err := f.db.Exec(
		`INSERT INTO artifacts (id, run_id, created_at) VALUES (?, ?, ?)`,
		id, runID, now,
	)
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return id, nil
}

// Get returns the HTML content and runID for an artifact id.
func (f *FileStore) Get(id string) (string, string, error) {
	var runID string
	err := f.db.QueryRow(`SELECT run_id FROM artifacts WHERE id = ?`, id).Scan(&runID)
	if err == sql.ErrNoRows {
		return "", "", fmt.Errorf("artifact not found")
	}
	if err != nil {
		return "", "", err
	}

	data, err := os.ReadFile(filepath.Join(f.dir, id+".html"))
	if err != nil {
		return "", "", err
	}
	return string(data), runID, nil
}
