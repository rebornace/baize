package store

import (
	"strings"
	"testing"
)

func TestVerifySchemaColumns(t *testing.T) {
	present := map[string]map[string]bool{
		"runs": {"id": true, "output": true},
	}
	ok := []schemaRequirement{{Table: "runs", Columns: []string{"id", "output"}}}
	if err := verifySchemaColumns(present, ok); err != nil {
		t.Fatalf("complete schema: %v", err)
	}

	missingTable := []schemaRequirement{{Table: "events", Columns: []string{"id"}}}
	err := verifySchemaColumns(present, missingTable)
	if err == nil || !strings.Contains(err.Error(), `table "events" is missing`) {
		t.Fatalf("missing table err=%v", err)
	}

	missingColumn := []schemaRequirement{{Table: "runs", Columns: []string{"id", "output", "conversation_id"}}}
	err = verifySchemaColumns(present, missingColumn)
	if err == nil || !strings.Contains(err.Error(), `missing column "conversation_id"`) {
		t.Fatalf("missing column err=%v", err)
	}
	if !strings.Contains(err.Error(), "back up the database") {
		t.Fatalf("error is not actionable: %v", err)
	}
}
