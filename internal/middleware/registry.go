package middleware

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// DriverFactory constructs a Middleware from options.
type DriverFactory func(ctx context.Context, opts Options) (*Middleware, error)

var (
	driversMu sync.RWMutex
	drivers   = map[string]DriverFactory{}
)

// RegisterDriver registers a middleware driver by name (called from init()).
func RegisterDriver(name string, f DriverFactory) {
	driversMu.Lock()
	defer driversMu.Unlock()
	drivers[name] = f
}

func lookupDriver(name string) (DriverFactory, error) {
	driversMu.RLock()
	defer driversMu.RUnlock()
	f, ok := drivers[name]
	if !ok {
		return nil, fmt.Errorf("unknown middleware driver %q (registered: %v)", name, ListDrivers())
	}
	return f, nil
}

// ListDrivers returns registered driver names, sorted.
func ListDrivers() []string {
	driversMu.RLock()
	defer driversMu.RUnlock()
	names := make([]string, 0, len(drivers))
	for n := range drivers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
