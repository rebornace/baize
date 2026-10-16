package schedule

import (
	"database/sql"
	"fmt"
)

// Open returns a Store for the given persistence driver.
func Open(driver string, db *sql.DB) (Store, error) {
	switch driver {
	case "memory":
		return NewMemoryStore(), nil
	case "sqlite":
		if db == nil {
			return nil, fmt.Errorf("sqlite schedule store requires db")
		}
		return OpenSQLite(db)
	case "postgres":
		if db == nil {
			return nil, fmt.Errorf("postgres schedule store requires db")
		}
		return OpenPostgres(db)
	default:
		return nil, fmt.Errorf("unsupported schedule driver %q", driver)
	}
}
