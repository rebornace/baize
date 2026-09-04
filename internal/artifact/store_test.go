package artifact_test

import (
	"context"
	"errors"
	"os"
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
	_, _, err := as.Get(context.Background(), "art_missing")
	if !errors.Is(err, artifact.ErrNotFound) {
		t.Fatalf("want artifact.ErrNotFound for missing metadata row, got %v", err)
	}
}

// 元数据行存在但 blob 对象缺失（如手工删过对象、或 S3 对象过期/丢失）：
// blob 驱动返回 blob.ErrNotFound，artifact 层应映射为 artifact.ErrNotFound。
func TestGetBlobMissingMapsToNotFound(t *testing.T) {
	dir := t.TempDir()
	as := newTestStore(t, dir)
	id, err := as.PutHTML(context.Background(), "run_1", "<html>x</html>")
	if err != nil {
		t.Fatal(err)
	}
	// 删掉 blob 文件，保留 SQL 元数据行。
	if err := os.Remove(filepath.Join(dir, "artifacts", id+".html")); err != nil {
		t.Fatal(err)
	}
	_, _, err = as.Get(context.Background(), id)
	if !errors.Is(err, artifact.ErrNotFound) {
		t.Fatalf("want artifact.ErrNotFound when blob object is missing, got %v", err)
	}
}

// 注入一个对 Get 总是返回普通错误的 blob.Store：该错误既不是
// blob.ErrNotFound 也不是 SQL 无行，必须原样上抛、不得映射为 ErrNotFound。
// Put/Delete 委托给底层 memory 存储，保证 PutHTML 能正常落库。
type failingGetBlobStore struct {
	blob.Store
}

func (f *failingGetBlobStore) Get(_ context.Context, _ string) ([]byte, error) {
	return nil, errors.New("s3 upstream: 503 Service Unavailable")
}

func TestGetBlobUpstreamErrorIsNotNotFound(t *testing.T) {
	dir := t.TempDir()
	st, err := store.OpenSQLite(filepath.Join(dir, "b.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	mem, err := blob.Open(context.Background(), "memory", blob.Options{})
	if err != nil {
		t.Fatal(err)
	}
	as, err := artifact.NewStore(&failingGetBlobStore{Store: mem}, st)
	if err != nil {
		t.Fatal(err)
	}
	id, err := as.PutHTML(context.Background(), "run_1", "<html>x</html>")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = as.Get(context.Background(), id)
	if err == nil {
		t.Fatalf("want upstream error")
	}
	if errors.Is(err, artifact.ErrNotFound) {
		t.Fatalf("upstream error must not map to ErrNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "get artifact blob") {
		t.Fatalf("want error wrapped with context, got %v", err)
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
