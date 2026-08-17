package connector

import (
	"context"
	"sort"

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
