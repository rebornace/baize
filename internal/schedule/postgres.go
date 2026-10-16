package schedule

import "database/sql"

// OpenPostgres creates a SQLStore on db and ensures schema exists.
func OpenPostgres(db *sql.DB) (*SQLStore, error) {
	if db == nil {
		return nil, sql.ErrConnDone
	}
	return openPostgresSQLStore(db)
}
