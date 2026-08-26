package connector

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector/executecallback"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
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

	// mcp-only:
	mcpSession     *mcp.ClientSession
	mcpHTTPURL     string
	mcpHTTPHeaders map[string]string

	// enterprise execution callback (openapi / http plugin invoke path):
	callbackURL string

	// sidecar plugin callback_urls.event injection (Phase 2). Only the http
	// plugin invoke path consumes these; openapi/mcp invokers ignore them.
	callbackSigner     httpplugin.CallbackSigner
	callbackSecret     []byte
	callbackPublicBase string
	callbackTTL        time.Duration
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
	switch t.Source {
	case store.ToolSourceSpec, store.ToolSourceExtra:
		invoker = openapiInvokerClosure(ctx, t.Name)
		if ctx.inv != nil {
			for _, r := range ctx.inv.Tools {
				if r.Name == t.Name {
					securitySchemes = r.Security
					break
				}
			}
		}
	case store.ToolSourceMCP:
		invoker = mcpInvokerClosure(ctx, t.Name)
	default:
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
		var out openapi.InvokeResult
		var err error
		if ctx.callbackURL != "" {
			cbOut, cbErr := invokeEnterpriseCallback(ctx.callbackURL, c, name, args, overlay)
			out = cbOut
			err = cbErr
		} else {
			out, err = ctx.inv.InvokeWithHeaders(c, name, args, overlay)
		}
		if err != nil {
			return nil, true, err
		}
		if usedID != "" && ctx.identities != nil {
			_ = ctx.identities.Touch(conv, usedID)
		}
		maybeCaptureLogin(conv, ctx.identities, ctx.capture, name, out.Content, out.IsError)
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
		if ctx.callbackURL != "" {
			out, invErr := invokeEnterpriseCallback(ctx.callbackURL, c, name, args, overlay)
			if invErr != nil {
				return nil, true, invErr
			}
			if usedID != "" && ctx.identities != nil {
				_ = ctx.identities.Touch(conv, usedID)
			}
			maybeCaptureLogin(conv, ctx.identities, ctx.capture, name, out.Content, out.IsError)
			return out.Content, out.IsError, nil
		}
		out, invErr := ctx.client.Invoke(c, name, args, httpplugin.InvokeMeta{
			RunID:            identity.RunIDFrom(c),
			AgentID:          identity.AgentIDFrom(c),
			Headers:          overlay,
			CallbackEventURL: httpplugin.SignCallbackEventURL(ctx.callbackSigner, ctx.callbackSecret, ctx.callbackPublicBase, ctx.callbackTTL, identity.RunIDFrom(c)),
		})
		if invErr != nil {
			return nil, true, invErr
		}
		if usedID != "" && ctx.identities != nil {
			_ = ctx.identities.Touch(conv, usedID)
		}
		maybeCaptureLogin(conv, ctx.identities, ctx.capture, name, out.Content, out.IsError)
		return out.Content, out.IsError, nil
	}
}

// mcpInvokerClosure builds the conversation-aware invoker for an MCP tool row.
// stdio connectors reuse the pooled session; HTTP connectors connect per call.
func mcpInvokerClosure(ctx registerOneContext, name string) tool.Invoker {
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
		_ = overlay // MCP auth is configured on the connector, not via overlay.

		var result *mcp.CallToolResult
		var err error
		if ctx.mcpSession != nil {
			result, err = mcpbridge.CallTool(c, ctx.mcpSession, name, args)
		} else if ctx.mcpHTTPURL != "" {
			session, connErr := mcpbridge.ConnectHTTP(c, ctx.mcpHTTPURL, ctx.mcpHTTPHeaders)
			if connErr != nil {
				return nil, true, connErr
			}
			defer session.Close()
			result, err = mcpbridge.CallTool(c, session, name, args)
		} else {
			return nil, true, fmt.Errorf("mcp session not configured for tool %q", name)
		}
		if err != nil {
			return nil, true, err
		}
		if usedID != "" && ctx.identities != nil {
			_ = ctx.identities.Touch(conv, usedID)
		}
		return mcpbridge.ToolResultContent(result), result.IsError, nil
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

// CallbackConfig carries the sidecar callback injection pieces needed by
// RegisterOneFromConnector. Zero value => no callback_urls injected.
type CallbackConfig struct {
	Signer     httpplugin.CallbackSigner
	Secret     []byte
	PublicBase string
	TTL        time.Duration
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
//
// cb wires callback_urls.event injection for the http plugin invoke path;
// pass a zero CallbackConfig to disable injection.
func RegisterOneFromConnector(st store.Store, reg *tool.Registry, ids identity.Store, c store.Connector, t store.Tool, cb CallbackConfig) error {
	typ := strings.TrimSpace(c.Type)
	if typ == "" {
		typ = "openapi"
	}
	if typ == "http" && t.Source == store.ToolSourceExtra {
		return fmt.Errorf("plugin connector %q does not support extra tools", c.ID)
	}
	if typ == "mcp" && t.Source == store.ToolSourceExtra {
		return fmt.Errorf("mcp connector %q does not support extra tools", c.ID)
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
	var mcpSession *mcp.ClientSession
	var mcpHTTPURL string
	var mcpHTTPHeaders map[string]string
	var capture identity.CaptureConfig
	switch typ {
	case "http":
		client = httpplugin.NewClient(c.BaseURL)
		capture = CaptureDefaults(identity.CaptureConfig{
			ToolNameGlob:   c.Auth.Capture.ToolNameGlob,
			TokenJSONPaths: c.Auth.Capture.TokenJSONPaths,
			LabelJSONPaths: c.Auth.Capture.LabelJSONPaths,
			HeaderTemplate: c.Auth.Capture.HeaderTemplate,
			DefaultScheme:  c.Auth.Capture.DefaultScheme,
		})
	case "mcp":
		cfg := c.MCP
		transport := strings.TrimSpace(cfg.Transport)
		if transport == "" {
			transport = "stdio"
		}
		switch transport {
		case "stdio":
			env, err := mcpbridge.ResolveEnv(cfg.Env)
			if err != nil {
				return err
			}
			session, err := mcpPool.ConnectStdio(context.Background(), cfg.Command, cfg.Args, env)
			if err != nil {
				return fmt.Errorf("%w: %w", mcpbridge.ErrInvalidMCP, err)
			}
			mcpPool.CommitStdio(c.ID, session)
			mcpSession = session
		case "http":
			headers, err := mcpbridge.ResolveHeaders(cfg.Headers)
			if err != nil {
				return err
			}
			mcpHTTPURL = cfg.URL
			mcpHTTPHeaders = headers
		default:
			return fmt.Errorf("%w: unsupported mcp transport: %s", mcpbridge.ErrInvalidMCP, transport)
		}
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
		reg:                reg,
		id:                 c.ID,
		headers:            headers,
		authMode:           authMode,
		identities:         ids,
		resolver:           authresolve.OpenAPISecurityResolver{},
		inv:                inv,
		capture:            capture,
		client:             client,
		mcpSession:         mcpSession,
		mcpHTTPURL:         mcpHTTPURL,
		mcpHTTPHeaders:     mcpHTTPHeaders,
		callbackURL:        strings.TrimSpace(c.ExecutionCallbackURL),
		callbackSigner:     cb.Signer,
		callbackSecret:     cb.Secret,
		callbackPublicBase: cb.PublicBase,
		callbackTTL:        cb.TTL,
	}
	registerOne(rctx, t)
	return nil
}

func invokeEnterpriseCallback(callbackURL string, c context.Context, name string, args map[string]any, overlay map[string]string) (openapi.InvokeResult, error) {
	client := executecallback.NewClient(callbackURL)
	out, err := client.Invoke(c, name, args, executecallback.InvokeMeta{
		RunID:          identity.RunIDFrom(c),
		AgentID:        identity.AgentIDFrom(c),
		IdempotencyKey: identity.ToolCallIDFrom(c),
		Headers:        overlay,
	})
	if err != nil {
		return openapi.InvokeResult{}, err
	}
	return openapi.InvokeResult{Content: out.Content, IsError: out.IsError}, nil
}
