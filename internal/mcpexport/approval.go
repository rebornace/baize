package mcpexport

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/rebornace/baize/internal/store"
)

const exportApprovalInputID = "baize_export_approval"

const msgExportApprovalUnsupported = "【中文】此白泽工具需要客户端审批（MCP elicitation），当前客户端不支持。请勿用其它工具或变通方式重试。请改用支持 elicitation 的客户端，或请管理员仅在此导出 Key 上暴露无需审批的工具。\n\n[English] This Baize tool requires in-client approval (MCP elicitation), which this client does not support. Do not retry with different tools or workarounds. Use a client that supports elicitation, or ask an admin to expose only non-approval tools on this export key."

const msgExportApprovalDeclined = "【中文】用户已拒绝白泽工具审批。请勿用其它工具或变通方式重试。\n\n[English] User declined Baize tool approval. Do not retry with different tools or workarounds."

// ApprovalOpts configures export approval heuristics.
type ApprovalOpts struct {
	WriteHeuristicElicit bool // default true when using NeedsExportApproval from export invoke path
}

// NeedsExportApproval reports whether an export tool call must pass client elicitation before invoke.
func NeedsExportApproval(tool store.Tool, opts ApprovalOpts) bool {
	if tool.RequireApproval {
		return true
	}
	if tool.DestructiveHint != nil && *tool.DestructiveHint {
		return true
	}
	if !opts.WriteHeuristicElicit {
		return false
	}
	if methodNeedsWriteApproval(tool.Method) {
		return true
	}
	if tool.Source == store.ToolSourceMCP && IsMCPWriteTool(tool.Name, toolDescription(tool)) {
		return true
	}
	return false
}

func toolDescription(tool store.Tool) string {
	if tool.Description != "" {
		return tool.Description
	}
	return tool.Title
}

func methodNeedsWriteApproval(method string) bool {
	m := strings.ToUpper(strings.TrimSpace(method))
	if m == "" {
		return false
	}
	switch m {
	case "GET", "HEAD":
		return false
	default:
		return true
	}
}

func clientSupportsElicitation(ss *mcp.ServerSession) bool {
	ip := ss.InitializeParams()
	if ip == nil || ip.Capabilities == nil || ip.Capabilities.Elicitation == nil {
		return false
	}
	return true
}

func exportApprovalElicitParams(toolName string) *mcp.ElicitParams {
	return &mcp.ElicitParams{
		Mode: "form",
		Message: fmt.Sprintf(
			"【中文】是否允许运行白泽工具 %q？\n\n[English] Approve running Baize tool %q?",
			toolName, toolName,
		),
	}
}

func approvalErrorResult(message string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: message}},
		IsError: true,
	}
}

// gateExportApproval enforces the client elicitation HITL gate before tool invoke.
// When stop is true, the caller must return gateRes and must not invoke the tool.
func gateExportApproval(ctx context.Context, req *mcp.CallToolRequest, toolName string) (gateRes *mcp.CallToolResult, stop bool, err error) {
	_ = ctx
	ss, ok := req.GetSession().(*mcp.ServerSession)
	if !ok {
		return approvalErrorResult(msgExportApprovalUnsupported), true, nil
	}
	if !clientSupportsElicitation(ss) {
		return approvalErrorResult(msgExportApprovalUnsupported), true, nil
	}

	if req.Params != nil && req.Params.InputResponses != nil {
		if raw, ok := req.Params.InputResponses[exportApprovalInputID]; ok {
			er, ok := raw.(*mcp.ElicitResult)
			if !ok || er.Action != "accept" {
				return approvalErrorResult(msgExportApprovalDeclined), true, nil
			}
			return nil, false, nil
		}
	}

	return &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{
			exportApprovalInputID: exportApprovalElicitParams(toolName),
		},
	}, true, nil
}
