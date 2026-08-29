package channel

import (
	"fmt"
	"sort"
	"sync"
)

// Config is opaque channel open configuration (agent_id, creds_dir, etc.).
type Config map[string]string

// Factory constructs a Channel from Config.
type Factory func(cfg Config) (Channel, error)

var (
	registryMu sync.RWMutex
	factories  = map[string]Factory{}
)

// RegisterChannel registers a channel factory by name (database/sql-style).
// Typically called from plugin init() via blank import in cmd/baize.
func RegisterChannel(name string, factory Factory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if name == "" {
		panic("channel: empty channel name")
	}
	if factory == nil {
		panic("channel: nil factory for " + name)
	}
	factories[name] = factory
}

// List returns registered channel names in sorted order.
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(factories))
	for name := range factories {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Open constructs a Channel by registered name.
func Open(name string, cfg Config) (Channel, error) {
	registryMu.RLock()
	f, ok := factories[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown channel %q", name)
	}
	return f(cfg)
}

func resetRegistryForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	factories = map[string]Factory{}
}
