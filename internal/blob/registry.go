package blob

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// DriverFactory constructs a Store from options.
type DriverFactory func(ctx context.Context, opts Options) (Store, error)

var (
	driversMu sync.RWMutex
	drivers   = map[string]DriverFactory{}
)

// RegisterDriver registers a blob driver by name (called from init()).
// Registering the same name twice panics (programmer error).
func RegisterDriver(name string, f DriverFactory) {
	driversMu.Lock()
	defer driversMu.Unlock()
	if _, ok := drivers[name]; ok {
		panic(fmt.Sprintf("blob: driver %q already registered", name))
	}
	drivers[name] = f
}

func lookupDriver(name string) (DriverFactory, error) {
	driversMu.RLock()
	defer driversMu.RUnlock()
	f, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("unknown blob driver %q (registered: %v)", name, listDriversLocked())
	}
	return f, nil
}

// ListDrivers returns registered driver names, sorted.
func ListDrivers() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()
	return listDriversLocked()
}

// listDriversLocked returns registered driver names, sorted. The caller must
// hold driversMu (at least a read lock); it exists so locked code paths (e.g.
// lookupDriver's error branch) do not re-enter RLock, which would deadlock
// once a writer is waiting.
func listDriversLocked() []string {
	names := make([]string, 0, len(drivers))
	for n := range drivers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// Open builds a Store for the named driver.
func Open(ctx context.Context, driver string, opts Options) (Store, error) {
	f, err := lookupDriver(driver)
	if err != nil {
		return nil, err
	}
	return f(ctx, opts)
}
