package store

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func textTimestampCase(t *testing.T, st Store) {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	now := time.Now().UTC()
	caseID := "case-time-" + suffix
	if err := st.UpsertEvalCase(EvalCase{
		ID: caseID, Status: EvalCaseConfirmed, Protocol: "deterministic",
		Input: "x", CreatedAt: now, UpdatedAt: now.Add(time.Minute),
	}); err != nil {
		t.Fatal(err)
	}
	gotCase, err := st.GetEvalCase(caseID)
	if err != nil {
		t.Fatal(err)
	}
	if !gotCase.CreatedAt.Equal(now) || !gotCase.UpdatedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("eval case times: created=%s updated=%s", gotCase.CreatedAt, gotCase.UpdatedAt)
	}

	connectorID := "conn-time-" + suffix
	if err := st.PutCatalogRevision(ConnectorCatalogRevision{
		ConnectorID: connectorID, Revision: 1, At: now,
	}); err != nil {
		t.Fatal(err)
	}
	revs, err := st.ListCatalogRevisions(connectorID, 1)
	if err != nil || len(revs) != 1 {
		t.Fatalf("revisions=%+v err=%v", revs, err)
	}
	if !revs[0].At.Equal(now) {
		t.Fatalf("catalog revision at=%s want %s", revs[0].At, now)
	}

	invID := "inv-time-" + suffix
	if err := st.PutMCPExportInvocation(MCPExportInvocation{
		ID: invID, At: now, KeyID: "k", ToolName: "t",
	}); err != nil {
		t.Fatal(err)
	}
	invs, err := st.ListMCPExportInvocations(50)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, inv := range invs {
		if inv.ID == invID {
			found = true
			if !inv.At.Equal(now) {
				t.Fatalf("invocation at=%s want %s", inv.At, now)
			}
		}
	}
	if !found {
		t.Fatalf("invocation %s missing", invID)
	}
}

func TestParseStoreTimeLegacyPostgres(t *testing.T) {
	for _, s := range []string{
		"2026-11-25 02:10:39.302864+00",
		"2026-11-25 02:10:39+00",
		"2026-11-25 10:10:39.302864+08:00",
	} {
		if _, err := parseStoreTime(s); err != nil {
			t.Fatalf("parse %q: %v", s, err)
		}
	}
}

func TestTextTimestampsMemory(t *testing.T) { textTimestampCase(t, NewMemory()) }

func TestTextTimestampsSQLite(t *testing.T) {
	st, err := OpenSQLite(filepath.Join(t.TempDir(), "timestamps.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	textTimestampCase(t, st)
}

func TestTextTimestampsPostgres(t *testing.T) {
	dsn := os.Getenv("BAIZE_TEST_PG_DSN")
	if dsn == "" {
		t.Skip("BAIZE_TEST_PG_DSN not set")
	}
	st, err := OpenPostgres(dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	textTimestampCase(t, st)
}
