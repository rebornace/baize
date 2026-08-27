package store

import "fmt"

// Open creates a Store by driver name.
// Supported drivers: "memory" (path ignored) and "sqlite" (path is DB file).
func Open(driver, path string) (Store, error) {
	switch driver {
	case "memory", "":
		return NewMemory(), nil
	case "sqlite":
		return OpenSQLite(path)
	default:
		return nil, fmt.Errorf("unknown store driver %q", driver)
	}
}
