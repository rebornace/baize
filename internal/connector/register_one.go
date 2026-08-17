package connector

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

// registerOneContext carries the shared dependencies needed to register a
// single enabled catalog row into the Registry. The same struct is reused by
// Apply (initial registration) and by PATCH/POST tool endpoints (task 4) so
// the invoker closure is not duplicated.
type registerOneContext struct {
	reg        *tool.Registry
	id         string
	headers    map[string]string
	authMode   string
	identities identity.Store
	resolver   authresolve.Resolver

	// openapi-only:
	inv     *openapi.Invoker
	capture identity.CaptureConfig
	// requireApprovalMutating forces HITL on non-GET/HEAD/OPTIONS spec routes.
	requireApprovalMutating bool

	// plugin-only:
	client *httpplugin.Client
}

// registerOne registers a single enabled catalog row into the Registry with
// the appropriate invoker closure. Spec and extra rows on an openapi connector
// route through the shared openapi Invoker; plugin rows route through the
// sidecar client. Disabled rows must be skipped by the caller.
func registerOne(ctx registerOneContext, t store.Tool) {
	needApproval := t.RequireApproval
	if ctx.requireApprovalMutating && isMutatingMethod(t.Method) && t.Source == store.ToolSourceSpec {
		needApproval = true
	}
	// Login/capture tools are auth bootstrap, not business writes — skip
	// blanket HITL. Explicit require_approval entries still force approval.
	if !t.RequireApproval && identity.MatchToolName(ctx.capture.ToolNameGlob, t.Name) {
		needApproval = false
	}

	var securitySchemes []string
	var invoker tool.Invoker
	if t.Source == store.ToolSourceSpec || t.Source == store.ToolSourceExtra {
		invoker = openapiInvokerClosure(ctx, t.Name)
		if ctx.inv != nil {
			for _, r := range ctx.inv.Tools {
				if r.Name == t.Name {
					securitySchemes = r.Security
					break
				}
			}
		}
	} else {
		invoker = pluginInvokerClosure(ctx, t.Name)
	}

	ctx.reg.RegisterMeta(tool.Meta{
		Spec: llm.ToolSpec{
			Name:        t.Name,
			Description: t.Description,
			InputSchema: t.InputSchema,
		},
		ConnectorID:     ctx.id,
		OperationID:     t.OperationID,
		Method:          t.Method,
		Path:            t.Path,
		RequireLogin:    t.RequireLogin,
		SecuritySchemes: securitySchemes,
	}, invoker, needApproval)
}

// openapiInvokerClosure builds the conversation-aware invoker for an openapi
// (spec or extra) tool row. It mirrors the historical RegisterWithOpts closure
// so behavior (overlay resolution, login gating, capture) is preserved.
func openapiInvokerClosure(ctx registerOneContext, name string) tool.Invoker {
	return func(c context.Context, args map[string]any) (map[string]any, bool, error) {
		conv := identity.ConversationIDFrom(c)
		var overlay map[string]string
		if conv == "" {
			overlay = ctx.headers
			if ctx.authMode == "passthrough" {
				if h := identity.PassthroughHeadersFrom(c); len(h) > 0 {
					overlay = h
				} else {
					overlay = nil
				}
			}
		}
		var usedID string
		resOK := false
		if ctx.identities != nil && ctx.resolver != nil {
			force := identity.ForceIdentityIDFrom(c)
			var defaultHeaders map[string]string
			if conv == "" {
				defaultHeaders = ctx.headers
				if ctx.authMode == "passthrough" {
					if h := identity.PassthroughHeadersFrom(c); len(h) > 0 {
						defaultHeaders = h
					} else {
						defaultHeaders = nil
					}
				}
			}
			var routeSecurity []string
			if ctx.inv != nil {
				for _, r := range ctx.inv.Tools {
					if r.Name == name {
						routeSecurity = r.Security
						break
					}
				}
			}
			in := authresolve.ResolveInput{
				Identities:      ctx.identities.List(conv),
				SecuritySchemes: routeSecurity,
				DefaultHeaders:  defaultHeaders,
				ForceIdentityID: force,
			}
			res := ctx.resolver.Resolve(c, in)
			if res.OK {
				overlay = res.Headers
				usedID = res.IdentityID
				resOK = true
			} else if conv != "" {
				overlay = nil
			}
		}
		if conv != "" && ctx.reg.RequiresLogin(name) && (!resOK || len(overlay) == 0) {
			return tool.LoginRequiredContent(), true, nil
		}
		out, err := ctx.inv.InvokeWithHeaders(c, name, args, overlay)
		if err != nil {
			return nil, true, err
		}
		if usedID != "" && ctx.identities != nil {
			_ = ctx.identities.Touch(conv, usedID)
		}
		if conv != "" && !out.IsError && ctx.identities != nil && identity.MatchToolName(ctx.capture.ToolNameGlob, name) {
			if h, label, sub, claims, ok := identity.ExtractCredential(ctx.capture, out.Content); ok {
				_, _ = ctx.identities.Upsert(conv, identity.Identity{
					Label:             label,
					Scheme:            ctx.capture.DefaultScheme,
					Subject:           sub,
					CredentialHeaders: h,
					Source:            identity.SourceLoginCapture,
					ClaimsSummary:      claims,
					IsDefault:         true,
				})
			}
		}
		return out.Content, out.IsError, nil
	}
}

// pluginInvokerClosure builds the conversation-aware invoker for a plugin row.
// It mirrors the historical httpplugin.RegisterWithOpts closure (no capture).
func pluginInvokerClosure(ctx registerOneContext, name string) tool.Invoker {
	return func(c context.Context, args map[string]any) (map[string]any, bool, error) {
		conv := identity.ConversationIDFrom(c)
		var overlay map[string]string
		if conv == "" {
			overlay = ctx.headers
			if ctx.authMode == "passthrough" {
				if h := identity.PassthroughHeadersFrom(c); len(h) > 0 {
					overlay = h
				} else {
					overlay = nil
				}
			}
		}
		var usedID string
		resOK := false
		if ctx.identities != nil && ctx.resolver != nil {
			force := identity.ForceIdentityIDFrom(c)
			var defaultHeaders map[string]string
			if conv == "" {
				defaultHeaders = ctx.headers
				if ctx.authMode == "passthrough" {
					if h := identity.PassthroughHeadersFrom(c); len(h) > 0 {
						defaultHeaders = h
					} else {
						defaultHeaders = nil
					}
				}
			}
			in := authresolve.ResolveInput{
				Identities:      ctx.identities.List(conv),
				SecuritySchemes: nil,
				DefaultHeaders:  defaultHeaders,
				ForceIdentityID: force,
			}
			res := ctx.resolver.Resolve(c, in)
			if res.OK {
				overlay = res.Headers
				usedID = res.IdentityID
				resOK = true
			} else if conv != "" {
				overlay = nil
			}
		}
		if conv != "" && ctx.reg.RequiresLogin(name) && (!resOK || len(overlay) == 0) {
			return tool.LoginRequiredContent(), true, nil
		}
		out, invErr := ctx.client.Invoke(c, name, args, httpplugin.InvokeMeta{
			RunID:   identity.RunIDFrom(c),
			AgentID: identity.AgentIDFrom(c),
			Headers: overlay,
		})
		if invErr != nil {
			return nil, true, invErr
		}
		if usedID != "" && ctx.identities != nil {
			_ = ctx.identities.Touch(conv, usedID)
		}
		return out.Content, out.IsError, nil
	}
}

// listsFromTools aggregates the RequireLogin / RequireApproval tool-name lists
// from a merged catalog and returns them sorted. Used to echo the lists back
// onto the persisted Connector row.
func listsFromTools(tools []store.Tool) (login, approval []string) {
	for _, t := range tools {
		if t.RequireLogin {
			login = append(login, t.Name)
		}
		if t.RequireApproval {
			approval = append(approval, t.Name)
		}
	}
	sort.Strings(login)
	sort.Strings(approval)
	return
}

// RegisterOneFromConnector registers a single enabled catalog row into the
// Registry, reconstructing the registerOneContext from the persisted
// Connector + Tool. It is the exported entry point for task 4 PATCH/POST
// handlers in internal/api; those packages cannot call the unexported
// registerOne directly.
//
// The function is intentionally a thin wrapper: it rebuilds the same context
// Apply builds (resolve auth, load spec / build sidecar client, extend the
// openapi Invoker with extra routes) and then delegates to registerOne. The
// invoker closure is shared, not duplicated.
//
// For plugin (type=http) connectors, extra rows are rejected with an error to
// match Apply's defense-in-depth guard; callers must not register extras on
// plugin connectors.
func RegisterOneFromConnector(st store.Store, reg *tool.Registry, ids identity.Store, c store.Connector, t store.Tool) error {
	typ := strings.TrimSpace(c.Type)
	if typ == "" {
		typ = "openapi"
	}
	if typ == "http" && t.Source == store.ToolSourceExtra {
		return fmt.Errorf("plugin connector %q does not support extra tools", c.ID)
	}

	authCfg := authcred.Config{
		Mode:        c.Auth.Mode,
		Static:      authcred.Static{Headers: c.Auth.Static.Headers},
		Passthrough: authcred.PassThru{Headers: c.Auth.Passthrough.Headers},
		VaultRef:    authcred.VaultRef{Headers: c.Auth.VaultRef.Headers},
	}
	headers, err := authcred.ResolveDefaults(authCfg)
	if err != nil {
		return fmt.Errorf("resolve connector auth: %w", err)
	}
	authMode := authcred.NormalizeMode(c.Auth.Mode)

	var inv *openapi.Invoker
	var client *httpplugin.Client
	var capture identity.CaptureConfig
	switch typ {
	case "http":
		client = httpplugin.NewClient(c.BaseURL)
	case "openapi":
		routes, err := openapi.LoadTools(c.Spec)
		if err != nil {
			return fmt.Errorf("%w: %w", openapi.ErrInvalidSpec, err)
		}
		capture = CaptureDefaults(identity.CaptureConfig{
			ToolNameGlob:   c.Auth.Capture.ToolNameGlob,
			TokenJSONPaths: c.Auth.Capture.TokenJSONPaths,
			LabelJSONPaths: c.Auth.Capture.LabelJSONPaths,
			HeaderTemplate: c.Auth.Capture.HeaderTemplate,
			DefaultScheme:  c.Auth.Capture.DefaultScheme,
		})
		if capture.DefaultScheme == "" {
			capture.DefaultScheme = UniqueSecurityScheme(routes)
		}
		inv = &openapi.Invoker{BaseURL: c.BaseURL, Tools: routes}
		// Extend with extra rows from the store so the shared invoker can
		// dispatch extras alongside spec routes (mirrors Apply).
		for _, et := range st.ListToolsByConnector(c.ID) {
			if et.Source != store.ToolSourceExtra {
				continue
			}
			inv.Tools = append(inv.Tools, openapi.ToolRoute{
				Name:        et.Name,
				Method:      et.Method,
				Path:        et.Path,
				InputSchema: et.InputSchema,
				Description: et.Description,
			})
		}
	default:
		return fmt.Errorf("unsupported connector type: %s", typ)
	}

	rctx := registerOneContext{
		reg:        reg,
		id:         c.ID,
		headers:    headers,
		authMode:   authMode,
		identities: ids,
		resolver:   authresolve.OpenAPISecurityResolver{},
		inv:        inv,
		capture:    capture,
		client:     client,
	}
	registerOne(rctx, t)
	return nil
}
