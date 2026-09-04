package artifact_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/blob"
	_ "github.com/rebornace/baize/internal/blob/file"
	_ "github.com/rebornace/baize/internal/blob/memory"
	"github.com/rebornace/baize/internal/store"
)

func newTestStore(t *testing.T, root string) artifact.Store {
	t.Helper()
	dbPath := filepath.Join(root, "b.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	blobs, err := blob.Open(context.Background(), "file", blob.Options{File: blob.FileOptions{RootDir: root}})
	if err != nil {
		t.Fatal(err)
	}
	as, err := artifact.NewStore(blobs, st)
	if err != nil {
		t.Fatal(err)
	}
	return as
}

func TestStorePutGetRoundTrip(t *testing.T) {
	as := newTestStore(t, t.TempDir())
	id, err := as.PutHTML(context.Background(), "run_1", "<html><body>ok</body></html>")
	if err != nil {
		t.Fatal(err)
	}
	html, runID, err := as.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run_1" || !strings.Contains(html, "ok") {
		t.Fatalf("got run=%s html=%s", runID, html)
	}
}

func TestGetNotFound(t *testing.T) {
	as := newTestStore(t, t.TempDir())
	if _, _, err := as.Get(context.Background(), "art_missing"); err == nil {
		t.Fatalf("want error for missing artifact")
	}
}

// recordingStore wraps a blob.Store and records Delete keys to verify rollback.
type recordingStore struct {
	blob.Store
	deleted []string
}

func (r *recordingStore) Delete(ctx context.Context, key string) error {
	r.deleted = append(r.deleted, key)
	return r.Store.Delete(ctx, key)
}

func TestPutHTMLRollsBackBlobOnMetadataFailure(t *testing.T) {
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	// 用 memory blob（包一层记录 Delete），构造完 artifact store 后关闭 DB，
	// 迫使 INSERT 元数据失败 → 应回滚已写入的字节。
	mem, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	rec := &recordingStore{Store: mem}
	as, err := artifact.NewStore(rec, st)
	if err != nil {
		t.Fatal(err)
	}
	_ = st.Close() // 此后任何 SQL Exec 都失败

	if _, err := as.PutHTML(context.Background(), "run_1", "<html>x</html>"); err == nil {
		t.Fatalf("want metadata insert failure")
	}
	if len(rec.deleted) != 1 || !strings.HasPrefix(rec.deleted[0], "artifacts/art_") {
		t.Fatalf("want exactly one rollback delete under artifacts/, got %v", rec.deleted)
	}
}
