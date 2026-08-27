package artifact_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/rebornace/baize/internal/artifact"
	"github.com/rebornace/baize/internal/store"
)

func TestFileStorePutGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "b.db")
	st, err := store.OpenSQLite(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	fs, err := artifact.NewFileStore(filepath.Join(dir, "artifacts"), st)
	if err != nil {
		t.Fatal(err)
	}
	id, err := fs.PutHTML("run_1", "<html><body>ok</body></html>")
	if err != nil {
		t.Fatal(err)
	}
	html, runID, err := fs.Get(id)
	if err != nil {
		t.Fatal(err)
	}
	if runID != "run_1" || !strings.Contains(html, "ok") {
		t.Fatalf("got run=%s html=%s", runID, html)
	}
}
