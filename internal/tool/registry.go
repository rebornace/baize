package tool

import (
	"context"
	"fmt"
	"sort"

	"github.com/rebornace/baize/internal/llm"
)

type Invoker func(ctx context.Context, args map[string]any) (content map[string]any, isError bool, err error)

type entry struct {
	spec    llm.ToolSpec
	invoker Invoker
}

type Registry struct {
	tools map[string]entry
}

func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]entry),
	}
}

func (r *Registry) Register(name string, inv Invoker) {
	r.tools[name] = entry{
		spec:    llm.ToolSpec{Name: name},
		invoker: inv,
	}
}

func (r *Registry) RegisterSpec(spec llm.ToolSpec, inv Invoker) {
	r.tools[spec.Name] = entry{spec: spec, invoker: inv}
}

func (r *Registry) Specs() []llm.ToolSpec {
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

func (r *Registry) Invoke(ctx context.Context, name string, args map[string]any) (map[string]any, bool, error) {
	e, ok := r.tools[name]
	if !ok {
		return nil, false, fmt.Errorf("unknown tool: %s", name)
	}
	return e.invoker(ctx, args)
}
