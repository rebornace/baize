package openapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// ErrToolConflict is returned when registering would overwrite another connector's tools.
var ErrToolConflict = errors.New("tool_conflict")

// ErrInvalidSpec is returned when the OpenAPI spec cannot be loaded
// or yields no usable operations.
var ErrInvalidSpec = errors.New("invalid_spec")

// RegisterConnector loads an OpenAPI connector into the store and tool registry.
func RegisterConnector(
	st store.Store,
	reg *tool.Registry,
	id, typ, specPath, baseURL string,
	requireApproval []string,
) (store.Connector, []tool.Info, error) {
	return RegisterWithOpts(st, reg, RegisterOpts{
		ID:              id,
		Type:            typ,
		SpecPath:        specPath,
		BaseURL:         baseURL,
		RequireApproval: requireApproval,
	})
}

// RegisterWithOpts registers a connector using RegisterOpts.
func RegisterWithOpts(st store.Store, reg *tool.Registry, opts RegisterOpts) (store.Connector, []tool.Info, error) {
	typ := opts.Type
	if typ == "" {
		typ = "openapi"
	}
	if typ != "openapi" {
		return store.Connector{}, nil, fmt.Errorf("unsupported connector type")
	}
	routes, err := LoadTools(opts.SpecPath)
	if err != nil {
		return store.Connector{}, nil, fmt.Errorf("%w: %w", ErrInvalidSpec, err)
	}
	if len(routes) == 0 {
		return store.Connector{}, nil, fmt.Errorf("%w: no usable operations", ErrInvalidSpec)
	}
	names := make([]string, len(routes))
	for i, r := range routes {
		names[i] = r.Name
	}
	if reg.WouldConflict(opts.ID, names) {
		return store.Connector{}, nil, ErrToolConflict
	}
	reg.UnregisterConnector(opts.ID)
	approval := map[string]bool{}
	for _, n := range opts.RequireApproval {
		approval[n] = true
	}
	login := map[string]bool{}
	for _, n := range opts.RequireLogin {
		login[n] = true
	}
	// Persist only names that still exist after this registration (drop disappeared).
	requireLogin := make([]string, 0, len(names))
	for _, n := range names {
		if login[n] {
			requireLogin = append(requireLogin, n)
		}
	}
	sort.Strings(requireLogin)
	// Headers are applied only via InvokeWithHeaders overlay so conversation
	// paths can omit connector defaults without inv.Headers falling through.
	inv := &Invoker{BaseURL: opts.BaseURL, Tools: routes}
	for _, route := range routes {
		route := route
		name := route.Name
		needApproval := approval[name]
		if opts.RequireApprovalMutating && isMutatingMethod(route.Method) {
			needApproval = true
		}
		// Login/capture tools are auth bootstrap, not business writes — skip blanket HITL.
		// Explicit require_approval entries still force approval when listed.
		if !approval[name] && identity.MatchToolName(opts.Capture.ToolNameGlob, name) {
			needApproval = false
		}
		needLogin := login[name]
		reg.RegisterMeta(tool.Meta{
			Spec: llm.ToolSpec{
				Name:        route.Name,
				Description: route.Description,
				InputSchema: route.InputSchema,
			},
			ConnectorID:     opts.ID,
			OperationID:     route.OperationID,
			Method:          route.Method,
			Path:            route.Path,
			RequireLogin:    needLogin,
			SecuritySchemes: route.Security,
		}, func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
			conv := identity.ConversationIDFrom(ctx)
			var overlay map[string]string
			if conv == "" {
				overlay = opts.Headers
				if opts.AuthMode == "passthrough" {
					if h := identity.PassthroughHeadersFrom(ctx); len(h) > 0 {
						overlay = h
					} else {
						overlay = nil
					}
				}
			}
			var usedID string
			resOK := false
			if opts.Identities != nil && opts.Resolver != nil {
				force := identity.ForceIdentityIDFrom(ctx)
				var defaultHeaders map[string]string
				if conv == "" {
					defaultHeaders = opts.Headers
					if opts.AuthMode == "passthrough" {
						if h := identity.PassthroughHeadersFrom(ctx); len(h) > 0 {
							defaultHeaders = h
						} else {
							defaultHeaders = nil
						}
					}
				}
				in := authresolve.ResolveInput{
					Identities:      opts.Identities.List(conv),
					SecuritySchemes: route.Security,
					DefaultHeaders:  defaultHeaders,
					ForceIdentityID: force,
				}
				res := opts.Resolver.Resolve(ctx, in)
				if res.OK {
					overlay = res.Headers
					usedID = res.IdentityID
					resOK = true
				} else if conv != "" {
					overlay = nil
				}
			}
			if conv != "" && reg.RequiresLogin(name) && (!resOK || len(overlay) == 0) {
				return tool.LoginRequiredContent(), true, nil
			}
			out, err := inv.InvokeWithHeaders(ctx, name, args, overlay)
			if err != nil {
				return nil, true, err
			}
			if usedID != "" && opts.Identities != nil {
				_ = opts.Identities.Touch(conv, usedID)
			}
			if conv != "" && !out.IsError && opts.Identities != nil && identity.MatchToolName(opts.Capture.ToolNameGlob, name) {
				if h, label, sub, claims, ok := identity.ExtractCredential(opts.Capture, out.Content); ok {
					_, _ = opts.Identities.Upsert(conv, identity.Identity{
						Label:             label,
						Scheme:            opts.Capture.DefaultScheme,
						Subject:           sub,
						CredentialHeaders: h,
						Source:            identity.SourceLoginCapture,
						ClaimsSummary:     claims,
						IsDefault:         true,
					})
				}
			}
			return out.Content, out.IsError, nil
		}, needApproval)
	}
	c := store.Connector{
		ID:              opts.ID,
		Type:            typ,
		Spec:            opts.SpecPath,
		BaseURL:         opts.BaseURL,
		RequireApproval: opts.RequireApproval,
		RequireLogin:    requireLogin,
		Auth:            opts.Auth,
	}
	st.UpsertConnector(c)
	return c, filterInfos(reg, opts.ID), nil
}

func isMutatingMethod(method string) bool {
	switch strings.ToUpper(method) {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func filterInfos(reg *tool.Registry, connectorID string) []tool.Info {
	all := reg.List()
	out := make([]tool.Info, 0, len(all))
	for _, info := range all {
		if info.ConnectorID == connectorID {
			out = append(out, info)
		}
	}
	return out
}
