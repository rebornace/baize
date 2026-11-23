package tool

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/rebornace/baize/internal/llm"
)

// ArgsPrecheck validates tool arguments before HITL approval. Only tools that
// register a precheck are affected; others behave as before.
type ArgsPrecheck func(args map[string]any, now time.Time) error

type Invoker func(ctx context.Context, args map[string]any) (content map[string]any, isError bool, err error)

type Meta struct {
	Spec            llm.ToolSpec
	ConnectorID     string
	OperationID     string
	Method          string
	Path            string
	RequireLogin    bool
	SecuritySchemes []string
}

type Info struct {
	Name            string         `json:"name"`
	Description     string         `json:"description,omitempty"`
	ConnectorID     string         `json:"connector_id"`
	OperationID     string         `json:"operation_id,omitempty"`
	Method          string         `json:"method,omitempty"`
	Path            string         `json:"path,omitempty"`
	InputSchema     map[string]any `json:"input_schema,omitempty"`
	RequireApproval bool           `json:"require_approval"`
	RequireLogin    bool           `json:"require_login"`
}

type entry struct {
	spec            llm.ToolSpec
	invoker         Invoker
	requireApproval bool
	requireLogin    bool
	argsPrecheck    ArgsPrecheck
	securitySchemes []string
	connectorID     string
	operationID     string
	method          string
	path            string
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
	var precheck ArgsPrecheck
	if prev, ok := r.tools[spec.Name]; ok {
		precheck = prev.argsPrecheck
	}
	r.tools[spec.Name] = entry{
		spec:            spec,
		invoker:         inv,
		requireApproval: requireApproval,
		argsPrecheck:    precheck,
	}
}

// RegisterMeta registers a tool with connector metadata.
func (r *Registry) RegisterMeta(meta Meta, inv Invoker, requireApproval bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var precheck ArgsPrecheck
	if prev, ok := r.tools[meta.Spec.Name]; ok {
		precheck = prev.argsPrecheck
	}
	r.tools[meta.Spec.Name] = entry{
		spec:            meta.Spec,
		invoker:         inv,
		requireApproval: requireApproval,
		requireLogin:    meta.RequireLogin,
		argsPrecheck:    precheck,
		securitySchemes: meta.SecuritySchemes,
		connectorID:     meta.ConnectorID,
		operationID:     meta.OperationID,
		method:          meta.Method,
		path:            meta.Path,
	}
}

// List returns tool metadata sorted by name.
func (r *Registry) List() []Info {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Info, len(names))
	for i, name := range names {
		out[i] = r.infoFromEntry(r.tools[name])
	}
	return out
}

// Get returns tool Info by name.
func (r *Registry) Get(name string) (Info, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	if !ok {
		return Info{}, false
	}
	return r.infoFromEntry(e), true
}

func (r *Registry) infoFromEntry(e entry) Info {
	return Info{
		Name:            e.spec.Name,
		Description:     e.spec.Description,
		ConnectorID:     e.connectorID,
		OperationID:     e.operationID,
		Method:          e.method,
		Path:            e.path,
		InputSchema:     e.spec.InputSchema,
		RequireApproval: e.requireApproval,
		RequireLogin:    e.requireLogin,
	}
}

// WouldConflict reports whether any name is already registered to a different connector.
func (r *Registry) WouldConflict(connectorID string, names []string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, name := range names {
		e, ok := r.tools[name]
		if !ok {
			continue
		}
		if e.connectorID != "" && e.connectorID != connectorID {
			return true
		}
	}
	return false
}

// UnregisterConnector removes all tools belonging to the given connector.
func (r *Registry) UnregisterConnector(connectorID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, e := range r.tools {
		if e.connectorID == connectorID {
			delete(r.tools, name)
		}
	}
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
		s := r.tools[name].spec
		// Surface the owning connector for two-level routing. Prefer the
		// connectorID recorded via RegisterMeta; fall back to a Source set
		// directly on the spec (RegisterSpecApproved) so both registration
		// paths preserve ownership.
		e := r.tools[name]
		if src := e.connectorID; src != "" {
			s.Source = src
		}
		if e.method != "" {
			s.Method = e.method
		}
		if e.path != "" {
			s.Path = e.path
		}
		s.RequireLogin = e.requireLogin
		out[i] = s
	}
	return out
}

// RegisterArgsPrecheck attaches optional argument validation before HITL.
func (r *Registry) RegisterArgsPrecheck(name string, fn ArgsPrecheck) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.tools[name]
	if !ok {
		return fmt.Errorf("unknown tool: %s", name)
	}
	e.argsPrecheck = fn
	r.tools[name] = e
	return nil
}

// PrecheckArgs runs a registered precheck when present. The second return is
// false when the tool has no precheck.
func (r *Registry) PrecheckArgs(name string, args map[string]any, now time.Time) (error, bool) {
	r.mu.RLock()
	e, ok := r.tools[name]
	fn := e.argsPrecheck
	r.mu.RUnlock()
	if !ok || fn == nil {
		return nil, false
	}
	return fn(args, now), true
}

// RequiresApproval reports whether the named tool must be approved before Invoke.
func (r *Registry) RequiresApproval(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	return ok && e.requireApproval
}

// RequiresLogin reports whether the named tool requires session login before Invoke.
func (r *Registry) RequiresLogin(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	return ok && e.requireLogin
}

// SecuritySchemes returns the OpenAPI security scheme names stored for the tool.
func (r *Registry) SecuritySchemes(name string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.tools[name]
	if !ok {
		return nil
	}
	return e.securitySchemes
}

// SetRequireLogin toggles require_login for a registered tool.
func (r *Registry) SetRequireLogin(name string, requireLogin bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.tools[name]
	if !ok {
		return fmt.Errorf("unknown tool: %s", name)
	}
	e.requireLogin = requireLogin
	r.tools[name] = e
	return nil
}

// SetRequireApproval toggles require_approval for a registered tool.
func (r *Registry) SetRequireApproval(name string, requireApproval bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.tools[name]
	if !ok {
		return fmt.Errorf("unknown tool: %s", name)
	}
	e.requireApproval = requireApproval
	r.tools[name] = e
	return nil
}

// SetDescription updates the description for a registered tool in place.
func (r *Registry) SetDescription(name, description string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.tools[name]
	if !ok {
		return fmt.Errorf("unknown tool: %s", name)
	}
	e.spec.Description = description
	r.tools[name] = e
	return nil
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
