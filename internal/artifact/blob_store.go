package artifact

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/rebornace/baize/internal/blob"
	"github.com/rebornace/baize/internal/dbutil"
	"github.com/rebornace/baize/internal/store"
)

const (
	artifactKeyPrefix   = "artifacts/"
	artifactContentType = "text/html; charset=utf-8"
)

const sqliteArtifactsSchema = `
CREATE TABLE IF NOT EXISTS artifacts (
  id TEXT PRIMARY KEY,
  run_id TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
`

// BlobStore stores HTML bytes in a blob.Store and metadata in SQL.
type BlobStore struct {
	blobs    blob.Store
	db       *sql.DB
	postgres bool
}

var _ Store = (*BlobStore)(nil)

// NewStore creates an artifact Store backed by blobs for bytes and the SQL
// backend for metadata.
func NewStore(blobs blob.Store, backend store.SQLBackend) (*BlobStore, error) {
	if blobs == nil {
		return nil, fmt.Errorf("blob store is nil")
	}
	if backend == nil {
		return nil, fmt.Errorf("sql backend is nil")
	}
	db := backend.DB()
	if db == nil {
		return nil, fmt.Errorf("sql db is nil")
	}
	postgres := backend.Dialect() == store.DialectPostgres
	schema := sqliteArtifactsSchema
	if postgres {
		schema = dbutil.RebindPostgres(schema)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate artifacts schema: %w", err)
	}
	return &BlobStore{blobs: blobs, db: db, postgres: postgres}, nil
}

func (s *BlobStore) q(query string) string {
	if s.postgres {
		return dbutil.RebindPostgres(query)
	}
	return query
}

func (s *BlobStore) key(id string) string { return artifactKeyPrefix + id + ".html" }

// PutHTML writes html to the blob store and records its association with runID
// in SQL. On metadata failure the just-written bytes are rolled back.
func (s *BlobStore) PutHTML(ctx context.Context, runID string, html string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := "art_" + hex.EncodeToString(b)
	key := s.key(id)

	if err := s.blobs.Put(ctx, key, []byte(html), artifactContentType); err != nil {
		return "", fmt.Errorf("put artifact blob: %w", err)
	}
	if _, err := s.db.ExecContext(ctx,
		s.q(`INSERT INTO artifacts (id, run_id, created_at) VALUES (?, ?, ?)`),
		id, runID, time.Now().Unix()); err != nil {
		if delErr := s.blobs.Delete(context.WithoutCancel(ctx), key); delErr != nil {
			log.Printf("artifact: rollback delete %s after metadata failure: %v", key, delErr)
		}
		return "", fmt.Errorf("record artifact metadata: %w", err)
	}
	return id, nil
}

// Get returns the HTML content and runID for an artifact id.
func (s *BlobStore) Get(ctx context.Context, id string) (string, string, error) {
	var runID string
	err := s.db.QueryRowContext(ctx, s.q(`SELECT run_id FROM artifacts WHERE id = ?`), id).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", fmt.Errorf("artifact %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", "", fmt.Errorf("query artifact metadata: %w", err)
	}
	data, err := s.blobs.Get(ctx, s.key(id))
	if err != nil {
		if errors.Is(err, blob.ErrNotFound) {
			return "", "", fmt.Errorf("artifact %s: %w", id, ErrNotFound)
		}
		return "", "", fmt.Errorf("get artifact blob: %w", err)
	}
	return string(data), runID, nil
}
