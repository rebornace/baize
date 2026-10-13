package mcpexport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/hostedlogin"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type ctxKey int

const ctxKeyExportAuth ctxKey = 1

type exportAuth struct {
	Key      store.MCPExportKey
	Identity store.MCPExportIdentity
}

// ServerDeps wires store, tool registry, and identity store into the export HTTP facade.
type ServerDeps struct {
	Store          store.Store
	Registry       *tool.Registry
	Identities     identity.Store
	HostedSessions hostedlogin.Sessions
	PublicBaseURL  string
	Enabled        bool
}

// NewHTTPHandler returns a Streamable HTTP MCP handler with Bearer key auth.
// When Enabled is false, every request returns 503. Missing/invalid/revoked keys return 401.
func NewHTTPHandler(deps ServerDeps) http.Handler {
	stream := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		return buildExportServer(r.Context(), deps)
	}, &mcp.StreamableHTTPOptions{Stateless: true})

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !deps.Enabled {
			http.Error(w, "mcp export disabled", http.StatusServiceUnavailable)
			return
		}
		auth, err := authenticateExport(r, deps.Store)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyExportAuth, auth)
		stream.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authenticateExport(r *http.Request, st store.Store) (exportAuth, error) {
	raw := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return exportAuth{}, errors.New("missing bearer token")
	}
	plaintext := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
	if plaintext == "" {
		return exportAuth{}, errors.New("empty bearer token")
	}
	key, err := st.LookupMCPExportKeyByHash(HashKey(plaintext))
	if err != nil || key == nil {
		return exportAuth{}, errors.New("unknown or revoked key")
	}
	identity, err := st.GetMCPExportIdentity(key.IdentityID)
	if err != nil {
		return exportAuth{}, err
	}
	return exportAuth{Key: *key, Identity: identity}, nil
}

func exportAuthFrom(ctx context.Context) (exportAuth, bool) {
	v, ok := ctx.Value(ctxKeyExportAuth).(exportAuth)
	return v, ok
}

func buildExportServer(ctx context.Context, deps ServerDeps) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "baize-mcp-export", Version: "v0"}, nil)
	auth, ok := exportAuthFrom(ctx)
	if !ok {
		return server
	}

	server.AddReceivingMiddleware(exportListFilterMiddleware(deps, auth))

	registerExportWorkflowPrompt(server)

	for _, t := range deps.Store.ListTools() {
		toolRow := t
		opts := policyOptsFor(deps.Store, toolRow.ConnectorID)
		if !AllowExportToolGuards(toolRow, opts) {
			continue
		}
		schema := toolRow.InputSchema
		if schema == nil {
			schema = map[string]any{"type": "object"}
		}
		desc := toolRow.Description
		if desc == "" {
			desc = toolRow.Title
		}
		name := toolRow.Name
		server.AddTool(&mcp.Tool{
			Name:        name,
			Description: desc,
			InputSchema: schema,
		}, func(callCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return invokeExportTool(callCtx, deps, auth, toolRow, req)
		})
	}
	return server
}

func registerExportWorkflowPrompt(server *mcp.Server) {
	body := ExportWorkflowSkillMarkdown()
	server.AddPrompt(&mcp.Prompt{
		Name:        "baize_export_workflow",
		Title:       "Baize export workflow",
		Description: "How to use Baize MCP export tools safely (approval, auth, errors).",
	}, func(_ context.Context, _ *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Messages: []*mcp.PromptMessage{{
				Role: "user",
				Content: &mcp.TextContent{
					Text: body,
				},
			}},
		}, nil
	})
}

func policyOptsFor(st store.Store, connectorID string) PolicyOpts {
	if connectorID == "" {
		return PolicyOpts{}
	}
	c, err := st.GetConnector(connectorID)
	if err != nil {
		return PolicyOpts{}
	}
	return PolicyOpts{DBReadonlyConnector: c.MCP.ExportDBReadonly}
}

func invokeExportTool(
	ctx context.Context,
	deps ServerDeps,
	auth exportAuth,
	toolRow store.Tool,
	req *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	name := toolRow.Name
	opts := policyOptsFor(deps.Store, toolRow.ConnectorID)
	key, err := deps.Store.GetMCPExportKey(auth.Key.ID)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "tool not exported"}},
			IsError: true,
		}, nil
	}
	if !AllowExportForKey(toolRow, opts, key) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "tool not exported"}},
			IsError: true,
		}, nil
	}

	if NeedsExportApproval(toolRow, ApprovalOpts{WriteHeuristicElicit: true}) {
		gateRes, stop, err := gateExportApproval(ctx, req, name)
		if err != nil {
			return nil, err
		}
		if stop {
			return gateRes, nil
		}
	}

	args := map[string]any{}
	if len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return &mcp.CallToolResult{
				Content: []mcp.Content{&mcp.TextContent{Text: "invalid arguments"}},
				IsError: true,
			}, nil
		}
	}

	exportID, err := resolveExportIdentity(deps.Store, key, toolRow.ConnectorID, "")
	if err != nil {
		return authExpiredForCall(ctx, deps, auth, toolRow, req), nil
	}
	callAuth := auth
	callAuth.Identity = exportID
	invokeCtx, err := InvokeContext(ctx, deps.Identities, exportID)
	if err != nil {
		return nil, err
	}
	content, isError, err := deps.Registry.Invoke(invokeCtx, name, args)
	if err != nil {
		if IsAuthFailure(nil, false, err) {
			return authExpiredForCall(ctx, deps, callAuth, toolRow, req), nil
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			IsError: true,
		}, nil
	}
	if isError && IsAuthFailure(content, isError, err) {
		return authExpiredForCall(ctx, deps, callAuth, toolRow, req), nil
	}
	text, _ := json.Marshal(content)
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: content,
		IsError:           isError,
	}, nil
}

func exportListFilterMiddleware(deps ServerDeps, auth exportAuth) mcp.Middleware {
	return func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil {
				return res, err
			}
			lr, ok := res.(*mcp.ListToolsResult)
			if !ok {
				return res, err
			}
			key, err := deps.Store.GetMCPExportKey(auth.Key.ID)
			if err != nil {
				lr.Tools = nil
				return lr, nil
			}
			filtered := lr.Tools[:0]
			for _, tl := range lr.Tools {
				toolRow, err := deps.Store.GetTool(tl.Name)
				if err != nil {
					continue
				}
				opts := policyOptsFor(deps.Store, toolRow.ConnectorID)
				if AllowExportForKey(toolRow, opts, key) {
					filtered = append(filtered, tl)
				}
			}
			lr.Tools = filtered
			return lr, nil
		}
	}
}

func authExpiredForCall(ctx context.Context, deps ServerDeps, auth exportAuth, toolRow store.Tool, req *mcp.CallToolRequest) *mcp.CallToolResult {
	hostedURL, sessionID, ok := hostedReauthURL(deps, auth, toolRow)
	if ok {
		maybeElicitHostedLoginURL(ctx, req, hostedURL, sessionID)
	}
	return authExpiredResult(auth.Identity.Name, hostedURL)
}
