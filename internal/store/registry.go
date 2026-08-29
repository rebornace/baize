package store

import (
	"fmt"
	"sort"
	"sync"
)

// OpenOptions carries driver-specific connection parameters.
type OpenOptions struct {
	SQLitePath string
	DSN        string
}

// DriverFactory opens a Store for the given options.
type DriverFactory func(opts OpenOptions) (Store, error)

var (
	driverMu sync.RWMutex
	drivers  = map[string]DriverFactory{}
)

// RegisterDriver registers a store driver by name. Typically called from driver
// package init() via blank import in cmd/baize.
func RegisterDriver(name string, factory DriverFactory) {
	driverMu.Lock()
	defer driverMu.Unlock()
	if name == "" {
		panic("store: empty driver name")
	}
	if factory == nil {
		panic("store: nil driver factory for " + name)
	}
	drivers[name] = factory
}

// ListDrivers returns registered driver names in sorted order.
func ListDrivers() []string {
	driverMu.RLock()
	defer driverMu.RUnlock()
	out := make([]string, 0, len(drivers))
	for name := range drivers {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func lookupDriver(name string) (DriverFactory, error) {
	driverMu.RLock()
	defer driverMu.RUnlock()
	f, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("unknown store driver %q", name)
	}
	return f, nil
}
