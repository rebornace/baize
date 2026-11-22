package store

import (
	"database/sql"
	"fmt"
)

// schemaRequirement lists columns the runtime queries directly. Adding a
// column with ALTER ... IF NOT EXISTS is safe; replacing or dropping one is
// not, and used to fail later at query time with a raw SQL error.
type schemaRequirement struct {
	Table   string
	Columns []string
}

var postgresRequiredColumns = []schemaRequirement{
	{Table: "runs", Columns: []string{"id", "agent_id", "input", "status", "output", "error", "created_at", "conversation_id"}},
	{Table: "events", Columns: []string{"id", "run_id", "type", "timestamp", "data_json"}},
	{Table: "messages", Columns: []string{"id", "conversation_id", "role", "content", "created_at"}},
	{Table: "connectors", Columns: []string{"id", "type", "spec", "base_url"}},
	{Table: "settings", Columns: []string{"key", "value_json"}},
	{Table: "conversation_meta", Columns: []string{"id", "owner_id", "source", "updated_at"}},
	{Table: "model_profiles", Columns: []string{"id", "name", "model"}},
}

// verifyPostgresSchema fails startup with an actionable message when an
// existing database is missing a column the runtime needs. It never alters or
// drops data.
func verifyPostgresSchema(db *sql.DB) error {
	rows, err := db.Query(`
		SELECT table_name, column_name
		FROM information_schema.columns
		WHERE table_schema = COALESCE(current_schema(), 'public')`)
	if err != nil {
		return fmt.Errorf("read postgres schema: %w", err)
	}
	defer rows.Close()

	present := map[string]map[string]bool{}
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			return fmt.Errorf("read postgres schema: %w", err)
		}
		if present[table] == nil {
			present[table] = map[string]bool{}
		}
		present[table][column] = true
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read postgres schema: %w", err)
	}
	return verifySchemaColumns(present, postgresRequiredColumns)
}

// verifySchemaColumns is the pure part of the check, kept separate so it can
// be tested without a live database.
func verifySchemaColumns(present map[string]map[string]bool, required []schemaRequirement) error {
	for _, req := range required {
		columns, ok := present[req.Table]
		if !ok {
			return fmt.Errorf("postgres schema incompatible: table %q is missing; back up the database and migrate manually", req.Table)
		}
		for _, column := range req.Columns {
			if !columns[column] {
				return fmt.Errorf("postgres schema incompatible: table %q is missing column %q; back up the database and migrate manually", req.Table, column)
			}
		}
	}
	return nil
}
