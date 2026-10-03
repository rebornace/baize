package store

import "fmt"

// Open creates a Store by driver name and legacy path (sqlite file path).
// Prefer OpenWithOptions for postgres and full configuration.
func Open(driver, path string) (Store, error) {
	return OpenWithOptions(driver, OpenOptions{SQLitePath: path})
}

// OpenWithOptions creates a Store using registered drivers.
func OpenWithOptions(driver string, opts OpenOptions) (Store, error) {
	switch driver {
	case "memory", "":
		return NewMemory(), nil
	case "sqlite":
		if opts.SQLitePath == "" {
			opts.SQLitePath = "./data/baize.db"
		}
		f, err := lookupDriver("sqlite")
		if err != nil {
			return nil, err
		}
		return f(opts)
	case "postgres":
		if opts.DSN == "" {
			return nil, fmt.Errorf("store.dsn is required for postgres driver")
		}
		f, err := lookupDriver("postgres")
		if err != nil {
			return nil, err
		}
		return f(opts)
	default:
		f, err := lookupDriver(driver)
		if err != nil {
			return nil, err
		}
		return f(opts)
	}
}
