package workspace

import (
	"context"
	"fmt"

	"github.com/rebornace/baize/internal/identity"
	"github.com/rebornace/baize/internal/llm"
	"github.com/rebornace/baize/internal/tool"
)

// ToolMeta pairs a tool spec with its invoker for registration.
type ToolMeta struct {
	Spec    llm.ToolSpec
	Invoker tool.Invoker
}

const workspaceToolHint = "This is a persistent, per-conversation file workspace. " +
	"Files you write and files the user uploads (under uploads/) persist across turns and runs " +
	"within this conversation. Paths are relative to the workspace root; absolute paths and '..' " +
	"are not allowed. Uploaded files are in uploads/."

func strArg(args map[string]any, key string) (string, bool) {
	v, ok := args[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok && s != ""
}

func fail(format string, a ...any) (map[string]any, bool, error) {
	return map[string]any{"error": fmt.Sprintf(format, a...)}, true, nil
}

// Tools returns the 5 built-in workspace tools for registration.
func Tools(svc *Service) []ToolMeta {
	listSpec := llm.ToolSpec{
		Name:        "list_files",
		Description: "列出工作区里的文件和目录（可选项 path 指定子目录，默认根目录；返回每个条目的名称和类型，上传的文件在 uploads/ 目录下）。List files and folders in your persistent workspace. Optional 'path' lists a subfolder (default: root). Returns entries with name and type (file/dir). Uploaded files are under uploads/. " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Optional folder path relative to workspace root"},
			},
		},
	}
	readSpec := llm.ToolSpec{
		Name:        "read_file",
		Description: "读取工作区里的 UTF-8 文本文件（例如 uploads/ 目录下上传的文档），返回文件内容（过大时截断）；查看图片请用 read_image。Read a UTF-8 text file from your workspace (e.g. an uploaded document under uploads/). Returns content (truncated if large). Use read_image for images. " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File path relative to workspace root"},
			},
			"required": []string{"path"},
		},
	}
	writeSpec := llm.ToolSpec{
		Name:        "write_file",
		Description: "在工作区写入（新建或覆盖）UTF-8 文本文件，例如保存笔记或供后续轮次使用的中间结果；父目录会自动创建，最大 256 KiB。Write (create or overwrite) a UTF-8 text file in your workspace, e.g. to save notes or intermediate results for later turns. Parent folders are created automatically. Max 256 KiB. " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path":    map[string]any{"type": "string", "description": "File path relative to workspace root"},
				"content": map[string]any{"type": "string", "description": "Full file content"},
			},
			"required": []string{"path", "content"},
		},
	}
	deleteSpec := llm.ToolSpec{
		Name:        "delete_file",
		Description: "删除工作区里的文件（幂等：删除不存在的文件也算成功）。Delete a file from your workspace. Idempotent (deleting a missing file succeeds). " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "File path relative to workspace root"},
			},
			"required": []string{"path"},
		},
	}
	imageSpec := llm.ToolSpec{
		Name:        "read_image",
		Description: "查看工作区里的图片文件（例如 uploads/ 目录下上传的图片），图片会附加到工具结果中供你查看；需要支持视觉的模型。View an image file from your workspace (e.g. an uploaded image under uploads/). The image is attached to this tool result for you to see. Requires a vision-capable model. " + workspaceToolHint,
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{"type": "string", "description": "Image path relative to workspace root"},
			},
			"required": []string{"path"},
		},
	}

	return []ToolMeta{
		{Spec: listSpec, Invoker: listInvoker(svc)},
		{Spec: readSpec, Invoker: readInvoker(svc)},
		{Spec: writeSpec, Invoker: writeInvoker(svc)},
		{Spec: deleteSpec, Invoker: deleteInvoker(svc)},
		{Spec: imageSpec, Invoker: readImageInvoker(svc)},
	}
}

func convFromCtx(ctx context.Context) (string, map[string]any, bool) {
	conv := identity.ConversationIDFrom(ctx)
	if conv == "" {
		m := map[string]any{"error": "workspace requires a conversation context"}
		return "", m, false
	}
	return conv, nil, true
}

func listInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		dir, _ := strArg(args, "path")
		entries, err := svc.ListFiles(ctx, conv, dir)
		if err != nil {
			return fail("%v", err)
		}
		return map[string]any{"path": dir, "entries": entries}, false, nil
	}
}

func readInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		p, ok := strArg(args, "path")
		if !ok {
			return fail("path is required")
		}
		content, err := svc.ReadFile(ctx, conv, p)
		if err != nil {
			return fail("%v", err)
		}
		return map[string]any{"path": p, "content": content}, false, nil
	}
}

func writeInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		p, ok := strArg(args, "path")
		if !ok {
			return fail("path is required")
		}
		content, _ := args["content"].(string)
		if err := svc.WriteFile(ctx, conv, p, content); err != nil {
			return fail("%v", err)
		}
		return map[string]any{"path": p, "bytes": len(content)}, false, nil
	}
}

func deleteInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		p, ok := strArg(args, "path")
		if !ok {
			return fail("path is required")
		}
		if err := svc.DeleteFile(ctx, conv, p); err != nil {
			return fail("%v", err)
		}
		return map[string]any{"path": p, "deleted": true}, false, nil
	}
}

func readImageInvoker(svc *Service) tool.Invoker {
	return func(ctx context.Context, args map[string]any) (map[string]any, bool, error) {
		conv, m, ok := convFromCtx(ctx)
		if !ok {
			return m, true, nil
		}
		p, ok := strArg(args, "path")
		if !ok {
			return fail("path is required")
		}
		part, ctype, err := svc.ReadImage(ctx, conv, p)
		if err != nil {
			return fail("%v", err)
		}
		return tool.WithImageParts(map[string]any{
			"path":         p,
			"content_type": ctype,
			"bytes":        len(part.ImageBytes),
			"note":         "image attached to this tool result",
		}, tool.ImageResult{Path: p, Part: part}), false, nil
	}
}
