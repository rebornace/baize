package mcp

import (
	"context"
	"encoding/json"
	"os/exec"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/store"
)

const requestTimeout = 30 * time.Second

var baizeClientInfo = &mcp.Implementation{Name: "baize", Version: "dev"}

// SessionPool holds one stdio MCP session per connector ID.
type SessionPool struct {
	mu       sync.Mutex
	sessions map[string]*mcp.ClientSession
}

// ConnectStdio opens a new stdio session without changing the pooled session.
// Call CommitStdio after discovery succeeds to replace the pooled entry.
func (p *SessionPool) ConnectStdio(ctx context.Context, command string, args, env []string) (*mcp.ClientSession, error) {
	cmd := exec.Command(command, args...)
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}

	client := mcp.NewClient(baizeClientInfo, nil)
	return client.Connect(ctx, &mcp.CommandTransport{Command: cmd}, nil)
}

// CommitStdio stores session in the pool, closing any prior session for connectorID.
func (p *SessionPool) CommitStdio(connectorID string, session *mcp.ClientSession) {
	if p.sessions == nil {
		p.sessions = make(map[string]*mcp.ClientSession)
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if existing, ok := p.sessions[connectorID]; ok {
		_ = existing.Close()
	}
	p.sessions[connectorID] = session
}

// OpenStdio connects and immediately commits the stdio session for connectorID.
func (p *SessionPool) OpenStdio(ctx context.Context, connectorID, command string, args, env []string) (*mcp.ClientSession, error) {
	session, err := p.ConnectStdio(ctx, command, args, env)
	if err != nil {
		return nil, err
	}
	p.CommitStdio(connectorID, session)
	return session, nil
}

// Close closes the stdio session for connectorID.
func (p *SessionPool) Close(connectorID string) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	session, ok := p.sessions[connectorID]
	if !ok {
		return nil
	}
	delete(p.sessions, connectorID)
	return session.Close()
}

// CloseAll closes every pooled stdio session.
func (p *SessionPool) CloseAll() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for id, session := range p.sessions {
		_ = session.Close()
		delete(p.sessions, id)
	}
}

// ListTools lists tools from session with a 30s timeout.
func ListTools(ctx context.Context, session *mcp.ClientSession) (*mcp.ListToolsResult, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	return session.ListTools(ctx, &mcp.ListToolsParams{})
}

// CallToolOpts carries Baize identity and callback metadata for MCP tools/call.
type CallToolOpts struct {
	RunID            string
	AgentID          string
	CallbackEventURL string
}

// CallToolWithOpts invokes a tool with optional _meta injection and a 30s timeout.
func CallToolWithOpts(ctx context.Context, session *mcp.ClientSession, name string, arguments any, opts CallToolOpts) (*mcp.CallToolResult, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	meta := BuildCallMeta(opts.RunID, opts.AgentID, opts.CallbackEventURL)
	params := &mcp.CallToolParams{Name: name, Arguments: arguments}
	if len(meta) > 0 {
		params.Meta = meta
	}
	return session.CallTool(ctx, params)
}

// CallTool invokes a tool with a 30s timeout.
func CallTool(ctx context.Context, session *mcp.ClientSession, name string, arguments any) (*mcp.CallToolResult, error) {
	return CallToolWithOpts(ctx, session, name, arguments, CallToolOpts{})
}

// DiscoverTools lists MCP tools and maps them to store.Tool rows.
func DiscoverTools(ctx context.Context, session *mcp.ClientSession, connectorID string) ([]store.Tool, error) {
	result, err := ListTools(ctx, session)
	if err != nil {
		return nil, err
	}
	if len(result.Tools) == 0 {
		return nil, ErrInvalidMCP
	}

	tools := make([]store.Tool, 0, len(result.Tools))
	for _, tool := range result.Tools {
		tools = append(tools, store.Tool{
			ConnectorID: connectorID,
			Name:        tool.Name,
			Source:      store.ToolSourceMCP,
			Enabled:     true,
			Title:       tool.Title,
			Description: tool.Description,
			InputSchema: inputSchemaMap(tool.InputSchema),
		})
	}
	return tools, nil
}

func inputSchemaMap(schema any) map[string]any {
	if schema == nil {
		return map[string]any{"type": "object"}
	}
	switch s := schema.(type) {
	case map[string]any:
		return s
	default:
		raw, err := json.Marshal(schema)
		if err != nil {
			return map[string]any{"type": "object"}
		}
		var out map[string]any
		if err := json.Unmarshal(raw, &out); err != nil {
			return map[string]any{"type": "object"}
		}
		return out
	}
}
