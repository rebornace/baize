package connector

import (
	"context"
	"fmt"
	"strings"

	"github.com/rebornace/baize/internal/authcred"
	"github.com/rebornace/baize/internal/authresolve"
	"github.com/rebornace/baize/internal/connector/httpplugin"
	mcpbridge "github.com/rebornace/baize/internal/connector/mcp"
	"github.com/rebornace/baize/internal/connector/openapi"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var mcpPool = &mcpbridge.SessionPool{}

// ApplyInput configures connector registration shared by bootstrap and PUT.
type ApplyInput struct {
	Store                   store.Store
	Registry                *tool.Registry
	Identities              identity.Store
	ID, Type, Spec, BaseURL string
	ExecutionCallbackURL    string
	RequireApproval         []string
	RequireApprovalMutating bool
	RequireLogin            *[]string // nil=从 Registry 保留同名；非 nil=整表（空切片=全公开）
	Auth                    store.ConnectorAuth
	MCP                     store.MCPConfig
}

// Apply resolves auth, discovers the connector's tools, merges them with the
// persisted catalog, then persists and registers only the enabled rows.
//
// Failure semantics: discovery, merge, and conflict checks happen before any
// Store or Registry mutation. On error the Store catalog and the Registry are
// left in the same state as before the call (no partial unregister, no dirty
// writes).
func Apply(in ApplyInput) (store.Connector, []tool.Info, error) {
	authCfg := authcred.Config{
		Mode:        in.Auth.Mode,
		Static:      authcred.Static{Headers: in.Auth.Static.Headers},
		Passthrough: authcred.PassThru{Headers: in.Auth.Passthrough.Headers},
		VaultRef:    authcred.VaultRef{Headers: in.Auth.VaultRef.Headers},
	}
	headers, err := authcred.ResolveDefaults(authCfg)
	if err != nil {
		return store.Connector{}, nil, fmt.Errorf("resolve connector auth: %w", err)
	}

	typ := strings.TrimSpace(in.Type)
	if typ == "" {
		typ = "openapi"
	}
	authMode := authcred.NormalizeMode(in.Auth.Mode)

	// Phase 1: discover tools without touching Store or Registry. On error,
	// return immediately so prior state is preserved.
	var discovered []store.Tool
	var inv *openapi.Invoker
	var client *httpplugin.Client
	var mcpSession *mcp.ClientSession
	var mcpHTTPURL string
	var mcpHTTPHeaders map[string]string
	var capture identity.CaptureConfig
	switch typ {
	case "http":
		if strings.TrimSpace(in.BaseURL) == "" {
			return store.Connector{}, nil, fmt.Errorf("connector.base_url is required")
		}
		cl := httpplugin.NewClient(in.BaseURL)
		if err := cl.Healthz(context.Background()); err != nil {
			return store.Connector{}, nil, err
		}
		tools, err := cl.ListTools(context.Background())
		if err != nil {
			return store.Connector{}, nil, err
		}
		for _, td := range tools {
			name := strings.TrimSpace(td.Name)
			if name == "" {
				continue
			}
			schema := td.InputSchema
			if len(schema) == 0 {
				schema = map[string]any{"type": "object"}
			}
			discovered = append(discovered, store.Tool{
				ConnectorID: in.ID,
				Name:        name,
				Source:      store.ToolSourcePlugin,
				Enabled:     true,
				Description: td.Description,
				InputSchema: schema,
			})
		}
		if len(discovered) == 0 {
			return store.Connector{}, nil, httpplugin.ErrInvalidPlugin
		}
		client = cl
	case "openapi":
		if strings.TrimSpace(in.Spec) == "" {
			return store.Connector{}, nil, fmt.Errorf("connector.spec is required")
		}
		routes, err := openapi.LoadTools(in.Spec)
		if err != nil {
			return store.Connector{}, nil, fmt.Errorf("%w: %w", openapi.ErrInvalidSpec, err)
		}
		if len(routes) == 0 {
			return store.Connector{}, nil, fmt.Errorf("%w: no usable operations", openapi.ErrInvalidSpec)
		}
		for _, r := range routes {
			discovered = append(discovered, store.Tool{
				ConnectorID: in.ID,
				Name:        r.Name,
				Source:      store.ToolSourceSpec,
				Enabled:     true,
				Description: r.Description,
				Method:      r.Method,
				Path:        r.Path,
				OperationID: r.OperationID,
				InputSchema: r.InputSchema,
			})
		}
		capture = CaptureDefaults(identity.CaptureConfig{
			ToolNameGlob:   in.Auth.Capture.ToolNameGlob,
			TokenJSONPaths: in.Auth.Capture.TokenJSONPaths,
			LabelJSONPaths: in.Auth.Capture.LabelJSONPaths,
			HeaderTemplate: in.Auth.Capture.HeaderTemplate,
			DefaultScheme:  in.Auth.Capture.DefaultScheme,
		})
		if capture.DefaultScheme == "" {
			capture.DefaultScheme = UniqueSecurityScheme(routes)
		}
		inv = &openapi.Invoker{BaseURL: in.BaseURL, Tools: routes}
	case "mcp":
		cfg := in.MCP
		transport := strings.TrimSpace(cfg.Transport)
		if transport == "" {
			transport = "stdio"
		}
		switch transport {
		case "stdio":
			if strings.TrimSpace(cfg.Command) == "" {
				return store.Connector{}, nil, fmt.Errorf("%w: mcp.command is required", mcpbridge.ErrInvalidMCP)
			}
			env, err := mcpbridge.ResolveEnv(cfg.Env)
			if err != nil {
				return store.Connector{}, nil, err
			}
			session, err := mcpPool.ConnectStdio(context.Background(), cfg.Command, cfg.Args, env)
			if err != nil {
				return store.Connector{}, nil, fmt.Errorf("%w: %w", mcpbridge.ErrInvalidMCP, err)
			}
			tools, err := mcpbridge.DiscoverTools(context.Background(), session, in.ID)
			if err != nil {
				_ = session.Close()
				return store.Connector{}, nil, err
			}
			discovered = tools
			mcpSession = session
		case "http":
			if strings.TrimSpace(cfg.URL) == "" {
				return store.Connector{}, nil, fmt.Errorf("%w: mcp.url is required", mcpbridge.ErrInvalidMCP)
			}
			headers, err := mcpbridge.ResolveHeaders(cfg.Headers)
			if err != nil {
				return store.Connector{}, nil, err
			}
			tools, err := mcpbridge.DiscoverToolsHTTP(context.Background(), cfg.URL, headers, in.ID)
			if err != nil {
				return store.Connector{}, nil, err
			}
			discovered = tools
			mcpHTTPURL = cfg.URL
			mcpHTTPHeaders = headers
		default:
			return store.Connector{}, nil, fmt.Errorf("%w: unsupported mcp transport: %s", mcpbridge.ErrInvalidMCP, transport)
		}
	default:
		return store.Connector{}, nil, fmt.Errorf("unsupported connector type: %s", typ)
	}

	// Phase 2: merge with the persisted catalog.
	existing := in.Store.ListToolsByConnector(in.ID)
	merged := MergeCatalog(MergeOpts{
		Existing:        existing,
		Discovered:      discovered,
		RequireLogin:    in.RequireLogin,
		RequireApproval: in.RequireApproval,
	})

	// Phase 2b: bake mutating HITL into spec rows when RequireApprovalMutating
	// is set. registerOne applies the same rule at registration time, but the
	// persisted row must also carry the flag so listsFromTools, the catalog
	// badge, and RegisterOneFromConnector (PATCH re-register, which does not
	// know RequireApprovalMutating) all observe the same state. Login/capture
	// tools (matching the capture glob with no explicit require_approval) are
	// exempt, mirroring registerOne's blanket-HITL skip.
	if in.RequireApprovalMutating {
		for i, t := range merged {
			if t.Source != store.ToolSourceSpec {
				continue
			}
			if !isMutatingMethod(t.Method) {
				continue
			}
			if !t.RequireApproval && identity.MatchToolName(capture.ToolNameGlob, t.Name) {
				continue
			}
			merged[i].RequireApproval = true
		}
	}

	// Phase 3: conflict check. Other connectors' tool names (including their
	// disabled rows) reserve the namespace; a merged row colliding with any of
	// them is a hard failure. Registry.WouldConflict alone is not enough because
	// disabled rows are not in the Registry.
	otherNames := map[string]bool{}
	for _, t := range in.Store.ListTools() {
		if t.ConnectorID != "" && t.ConnectorID != in.ID {
			otherNames[t.Name] = true
		}
	}
	for _, t := range merged {
		if otherNames[t.Name] {
			if mcpSession != nil {
				_ = mcpSession.Close()
			}
			return store.Connector{}, nil, openapi.ErrToolConflict
		}
	}

	if mcpSession != nil {
		mcpPool.CommitStdio(in.ID, mcpSession)
	}

	// Phase 4: persist. ReplaceConnectorTools swaps the connector's rows
	// atomically (best-effort for SQLite); UpsertConnector echoes the aggregated
	// RequireLogin / RequireApproval lists back onto the connector row.
	in.Store.ReplaceConnectorTools(in.ID, merged)
	login, approval := listsFromTools(merged)
	auth := in.Auth
	if typ == "openapi" {
		auth.Capture = store.CaptureAuth{
			ToolNameGlob:   capture.ToolNameGlob,
			TokenJSONPaths: capture.TokenJSONPaths,
			LabelJSONPaths: capture.LabelJSONPaths,
			HeaderTemplate: capture.HeaderTemplate,
			DefaultScheme:  capture.DefaultScheme,
		}
	} else {
		auth.Capture = store.CaptureAuth{}
	}
	c := store.Connector{
		ID:                   in.ID,
		Type:                 typ,
		Spec:                 in.Spec,
		BaseURL:              in.BaseURL,
		ExecutionCallbackURL: strings.TrimSpace(in.ExecutionCallbackURL),
		RequireApproval:      approval,
		RequireLogin:         login,
		Auth:                 auth,
	}
	if typ == "mcp" {
		c.MCP = in.MCP
	}
	in.Store.UpsertConnector(c)

	// Phase 5: register enabled rows. Unregister the connector's stale tools
	// first, then register only enabled merged rows through the shared
	// registerOne closure.
	in.Registry.UnregisterConnector(in.ID)
	if typ == "openapi" {
		// Extend the Invoker with extra rows converted to ToolRoutes so the
		// shared invoker can dispatch them alongside spec routes.
		for _, t := range merged {
			if t.Source != store.ToolSourceExtra {
				continue
			}
			inv.Tools = append(inv.Tools, openapi.ToolRoute{
				Name:        t.Name,
				Method:      t.Method,
				Path:        t.Path,
				InputSchema: t.InputSchema,
				Description: t.Description,
			})
		}
	}
	rctx := registerOneContext{
		reg:                     in.Registry,
		id:                      in.ID,
		headers:                 headers,
		authMode:                authMode,
		identities:              in.Identities,
		resolver:                authresolve.OpenAPISecurityResolver{},
		inv:                     inv,
		capture:                 capture,
		requireApprovalMutating: in.RequireApprovalMutating,
		client:                  client,
		mcpSession:              mcpSession,
		mcpHTTPURL:              mcpHTTPURL,
		mcpHTTPHeaders:          mcpHTTPHeaders,
		callbackURL:             strings.TrimSpace(in.ExecutionCallbackURL),
	}
	for _, t := range merged {
		if !t.Enabled {
			continue
		}
		// Defense-in-depth: a plugin (type=http) connector must not register
		// extra rows. MergeCatalog preserves extras verbatim regardless of
		// connector type, so without this guard an extra row on a plugin
		// connector would route to openapiInvokerClosure (which dereferences
		// the nil ctx.inv) and panic at invoke time. The invariant is also
		// enforced at the POST entry (task 4), but Apply must not rely on
		// upstream callers to keep the catalog clean.
		if (typ == "http" || typ == "mcp") && t.Source == store.ToolSourceExtra {
			continue
		}
		registerOne(rctx, t)
	}

	return c, filterInfosByConnector(in.Registry, in.ID), nil
}

// filterInfosByConnector returns the Registry entries belonging to the given
// connector, sorted by name (Registry.List already sorts).
func filterInfosByConnector(reg *tool.Registry, connectorID string) []tool.Info {
	all := reg.List()
	out := make([]tool.Info, 0, len(all))
	for _, info := range all {
		if info.ConnectorID == connectorID {
			out = append(out, info)
		}
	}
	return out
}

// isMutatingMethod reports whether the HTTP method mutates server state.
func isMutatingMethod(method string) bool {
	switch strings.ToUpper(method) {
	case "GET", "HEAD", "OPTIONS":
		return false
	default:
		return true
	}
}
