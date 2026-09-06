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

// Descriptor declares a channel's static metadata for the bootstrap layer.
type Descriptor struct {
	// Name is the channel type key (e.g. "weixin").
	Name string
	// Build constructs a Channel from opaque Config.
	Build func(Config) (Channel, error)
	// DefaultCredsDir is the per-channel settings/creds directory basename.
	DefaultCredsDir string
	// EnabledByDefault marks a built-in channel that must stay wired even when
	// a declarative config channels: section is present but does not list it.
	// This keeps partial declarative configs backward compatible (the default
	// channel is never silently disabled); an explicit enabled:false still
	// wins. Optional/additional channels leave this false.
	EnabledByDefault bool
	// DeclarativeOnly marks a channel type that can only be instantiated from
	// explicit declarative config instances (it requires per-instance opaque
	// config and/or supports multiple instances). It is never auto-wired once
	// per type in the legacy (no `channels:` section) path. Optional/add-on
	// channels that are fine to auto-wire leave this false.
	DeclarativeOnly bool
}

var (
	registryMu sync.RWMutex
	descs      = map[string]Descriptor{}
)

// Register registers a channel Descriptor (preferred over RegisterChannel).
// Typically called from a channel package's init() via blank import in cmd/baize.
func Register(desc Descriptor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if desc.Name == "" {
		panic("channel: empty channel name")
	}
	if desc.Build == nil {
		panic("channel: nil build for " + desc.Name)
	}
	descs[desc.Name] = desc
}

// RegisterChannel registers a bare factory (back-compat); metadata is empty.
func RegisterChannel(name string, factory Factory) {
	Register(Descriptor{Name: name, Build: factory})
}

// List returns registered channel names in sorted order.
func List() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(descs))
	for name := range descs {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// Describe returns the Descriptor for a channel name.
func Describe(name string) (Descriptor, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	d, ok := descs[name]
	return d, ok
}

// Descriptors returns all registered Descriptors sorted by Name.
func Descriptors() []Descriptor {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Descriptor, 0, len(descs))
	for _, d := range descs {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// Open constructs a Channel by registered name.
func Open(name string, cfg Config) (Channel, error) {
	registryMu.RLock()
	d, ok := descs[name]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown channel %q", name)
	}
	return d.Build(cfg)
}

// ResetForTest clears the registry. It is intended for tests that need a
// deterministic descriptor table (e.g. the bootstrap package's wiring tests).
func ResetForTest() {
	registryMu.Lock()
	defer registryMu.Unlock()
	descs = map[string]Descriptor{}
}
