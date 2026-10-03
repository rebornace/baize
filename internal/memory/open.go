package memory

import (
	"database/sql"
	"fmt"
)

// Open returns a Store for the given persistence driver.
// For sqlite, db must be non-nil (typically the shared baize SQL connection).
func Open(driver string, db *sql.DB) (Store, error) {
	switch driver {
	case "memory":
		return NewMemoryStore(), nil
	case "sqlite":
		if db == nil {
			return nil, fmt.Errorf("sqlite memory store requires db")
		}
		return OpenSQLite(db)
	case "postgres":
		if db == nil {
			return nil, fmt.Errorf("postgres memory store requires db")
		}
		return OpenPostgres(db)
	default:
		return nil, fmt.Errorf("unsupported memory driver %q", driver)
	}
}
