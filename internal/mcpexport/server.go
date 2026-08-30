package mcpexport

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/store"
	"github.com/rebornace/baize/internal/tool"
)

type ctxKey int

const ctxKeyExportIdentity ctxKey = 1

// ServerDeps wires store, tool registry, and identity store into the export HTTP facade.
type ServerDeps struct {
	Store      store.Store
	Registry   *tool.Registry
	Identities identity.Store
	Enabled    bool
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
		export, err := authenticateExport(r, deps.Store)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), ctxKeyExportIdentity, export)
		stream.ServeHTTP(w, r.WithContext(ctx))
	})
}

func authenticateExport(r *http.Request, st store.Store) (store.MCPExportIdentity, error) {
	raw := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(raw, prefix) {
		return store.MCPExportIdentity{}, errors.New("missing bearer token")
	}
	plaintext := strings.TrimSpace(strings.TrimPrefix(raw, prefix))
	if plaintext == "" {
		return store.MCPExportIdentity{}, errors.New("empty bearer token")
	}
	key, err := st.LookupMCPExportKeyByHash(HashKey(plaintext))
	if err != nil || key == nil {
		return store.MCPExportIdentity{}, errors.New("unknown or revoked key")
	}
	return st.GetMCPExportIdentity(key.IdentityID)
}

func exportIdentityFrom(ctx context.Context) (store.MCPExportIdentity, bool) {
	v, ok := ctx.Value(ctxKeyExportIdentity).(store.MCPExportIdentity)
	return v, ok
}

func buildExportServer(ctx context.Context, deps ServerDeps) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "baize-mcp-export", Version: "v0"}, nil)
	export, ok := exportIdentityFrom(ctx)
	if !ok {
		return server
	}

	for _, t := range deps.Store.ListTools() {
		toolRow := t
		opts := policyOptsFor(deps.Store, toolRow.ConnectorID)
		if !AllowExport(toolRow, opts) {
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
			return invokeExportTool(callCtx, deps, export, name, req)
		})
	}
	return server
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
	export store.MCPExportIdentity,
	name string,
	req *mcp.CallToolRequest,
) (*mcp.CallToolResult, error) {
	toolRow, err := deps.Store.GetTool(name)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "tool not found"}},
			IsError: true,
		}, nil
	}
	opts := policyOptsFor(deps.Store, toolRow.ConnectorID)
	if !AllowExport(toolRow, opts) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: "tool not exported"}},
			IsError: true,
		}, nil
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

	invokeCtx, err := InvokeContext(ctx, deps.Identities, export)
	if err != nil {
		return nil, err
	}
	content, isError, err := deps.Registry.Invoke(invokeCtx, name, args)
	if err != nil {
		return &mcp.CallToolResult{
			Content: []mcp.Content{&mcp.TextContent{Text: err.Error()}},
			IsError: true,
		}, nil
	}
	text, _ := json.Marshal(content)
	return &mcp.CallToolResult{
		Content:           []mcp.Content{&mcp.TextContent{Text: string(text)}},
		StructuredContent: content,
		IsError:           isError,
	}, nil
}
