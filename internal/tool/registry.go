package tool

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/rebornace/baize/internal/llm"
)

type Invoker func(ctx context.Context, args map[string]any) (content map[string]any, isError bool, err error)

type entry struct {
	spec            llm.ToolSpec
	invoker         Invoker
	requireApproval bool
}

type Registry struct {
	mu    sync.RWMutex
	tools map[string]entry
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]entry),
	}
}

func (r *Registry) Register(name string, inv Invoker) {
	r.RegisterSpecApproved(llm.ToolSpec{Name: name}, inv, false)
}

func (r *Registry) RegisterSpec(spec llm.ToolSpec, inv Invoker) {
	r.RegisterSpecApproved(spec, inv, false)
}

// RegisterSpecApproved registers a tool and whether Invoke requires HITL approval.
func (r *Registry) RegisterSpecApproved(spec llm.ToolSpec, inv Invoker, requireApproval bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[spec.Name] = entry{spec: spec, invoker: inv, requireApproval: requireApproval}
}

// Unregister removes a tool by name. No-op if missing.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.tools, name)
}

func (r *Registry) Specs() []llm.ToolSpec {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]llm.ToolSpec, len(names))
	for i, name := range names {
		out[i] = r.tools[name].spec
	}
	return out
}

// RequiresApproval reports whether the named tool must be approved before Invoke.
func (r *Registry) RequiresApproval(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	return ok && e.requireApproval
}

func (r *Registry) Invoke(ctx context.Context, name string, args map[string]any) (map[string]any, bool, error) {
	r.mu.RLock()
	e, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, false, fmt.Errorf("unknown tool: %s", name)
	}
	return e.invoker(ctx, args)
}
