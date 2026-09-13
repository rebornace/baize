# UI-EXPORT-DB-RO：MCP 导出只读开关实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 在 MCP 连接编辑弹窗增加「对外导出时按数据库只读筛选」复选框，随 `putConnector` 写入 `mcp.export_db_readonly`；回显与测例齐全。

**架构：** 纯前端接线既有后端字段；`McpFormValues` → `validateMcp` → `MCPConfig.export_db_readonly`；不改 Go 策略。

**技术栈：** React + Vitest；现有 `ConnectorEditorModal` / `connectorForms/mcp.ts`。提交中文 Conventional Commits。

规格：`docs/superpowers/specs/2026-09-13-ui-export-db-readonly-design.md`

---

## 文件结构

修改：

- `web/chat/src/api.ts` — `MCPConfig.export_db_readonly?`
- `web/chat/src/pages/connectorForms/mcp.ts` + `mcp.test.ts`
- `web/chat/src/components/settings/ConnectorEditorModal.tsx` + `.test.tsx`
- `web/chat/src/strings.ts` — CONNECTORS 文案
- `README.md` / `README.zh-CN.md`
- `docs/superpowers/notes/2026-09-13-spec-ledger.md` / 确认清单
- `docs/superpowers/specs/2026-09-12-webui-refresh-p3c-mcp-pages-design.md` §11 交叉引用
- `internal/ui/dist/**` — `npm run build`

---

## 任务 1：表单模型与校验（TDD）

**文件：** `connectorForms/mcp.ts`、`mcp.test.ts`、`api.ts`、`strings.ts`

- [ ] **步骤 1：失败测** — `validateMcp` 在 `exportDbReadonly: true` 时 `mcp.export_db_readonly === true`；`false` 时字段省略或为 false；`connectorToMcpForm` 回显

- [ ] **步骤 2：** `npm test -- mcp.test` FAIL

- [ ] **步骤 3：实现**

```ts
// McpFormValues 增加 exportDbReadonly: boolean
// validateMcp 成功分支：
mcp: {
  ...,
  ...(v.exportDbReadonly ? { export_db_readonly: true } : {}),
}
// connectorToMcpForm: exportDbReadonly: !!mcp?.export_db_readonly
// EMPTY / api.MCPConfig 同步
```

文案：`exportDbReadonly` / `exportDbReadonlyHint` 挂 `CONNECTORS`。

- [ ] **步骤 4：** PASS

- [ ] **步骤 5：Commit** `feat(ui): MCP 表单支持 export_db_readonly`

---

## 任务 2：编辑弹窗接线

**文件：** `ConnectorEditorModal.tsx`、`.test.tsx`

- [ ] **步骤 1：失败测** — MCP 表单勾选后第一步保存，`onSaveInfo` 的 `mcp` 含 `export_db_readonly: true`；编辑初始值勾选回显

- [ ] **步骤 2–3：** 在 transport 字段后加 checkbox（`ui-checkbox-row`），绑定 `mcpForm.exportDbReadonly`；`EMPTY_MCP_FORM` 默认 false

- [ ] **步骤 4：** `npm test -- ConnectorEditorModal` PASS

- [ ] **步骤 5：Commit** `feat(ui): MCP 编辑弹窗导出只读开关`

---

## 任务 3：文档、账本、构建

- [ ] README / README.zh-CN MCP 导出节：一句说明连接器可开 `export_db_readonly`（UI 路径）
- [ ] ledger §4 WebUI 仍欠表去掉该项或标已交付；确认清单 UI-EXPORT-DB-RO 就绪列改已交付
- [ ] P3-C §11：`export_db_readonly` UI 行改为「已由 UI-EXPORT-DB-RO 承接」+ 链规格
- [ ] `npm run build`；提交 dist
- [ ] Commit `docs: UI-EXPORT-DB-RO 收口`
