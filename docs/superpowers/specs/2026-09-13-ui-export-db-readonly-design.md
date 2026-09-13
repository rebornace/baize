# UI-EXPORT-DB-RO：MCP 连接「对外导出只读」开关

- 日期：2026-09-13
- 状态：已批准（2026-09-13）
- 归属：开源首版史诗 **UI-EXPORT-DB-RO**（确认清单合并策略 A；**不**与 MCP OAuth 并计划）
- 承接：`2026-09-12-webui-refresh-p3c-mcp-pages-design.md` §11 曾 defer 的 `export_db_readonly` UI 写入

## 1. 目标 / 非目标

### 1.1 目标

在 **MCP 连接**编辑弹窗中，管理员可开关「对外导出时按数据库只读筛选」，随连接器 `PUT` 一并写入 `mcp.export_db_readonly`。使数据库类第三方 MCP 在 **MCP 导出**（`/v0/mcp/export`）面上可运维，无需改 YAML。

### 1.2 非目标

- 不改 `internal/mcpexport` 策略语义（`AllowExport` / `IsMCPWriteTool` 已交付）
- 不在「对外提供能力」页再放一份开关
- 不自动推断「是否像数据库 MCP」
- 不做 MCP OAuth、capture、OpenAPI 连接器的同类字段
- 不改变本机对话/Run 对 MCP 工具的可用性（本开关只影响导出面）

## 2. 背景

产品可作 MCP **客户端**接入第三方服务（设置 → MCP 连接），也可作 MCP **服务端**把目录子集导出给个人 Agent。`export_db_readonly` 挂在 `store.MCPConfig` 上，导出策略在 `DBReadonlyConnector` 时对 `source=mcp` 工具施加更严筛选；写类工具仍永不导出。P3-C 未做 UI 写入，本史诗补齐。

## 3. UX

| 项 | 约定 |
|----|------|
| 入口 | MCP 连接新建/编辑弹窗第一步（与 transport / command|url 同表单） |
| 控件 | 复选框；stdio 与 http 共用 |
| 标签 | 「对外导出时按数据库只读筛选」 |
| Hint | 仅约束 MCP 导出；对话里仍可用该连接器已启用工具；建议数据库类 MCP 开启 |
| 默认 | 新建 `false`；编辑回显库内值 |
| 保存 | 与现有 `validateMcp` → `onSaveInfo` → `putConnector` 同路径，body 含 `mcp.export_db_readonly` |

运营只读角色：与现有 MCP 编辑一致（不可改则控件 disabled）。

## 4. 数据契约（只读引用）

后端已有，本刀不改 Go 路由：

```json
{
  "type": "mcp",
  "mcp": {
    "transport": "stdio|http",
    "command": "...",
    "export_db_readonly": true
  }
}
```

- 字段：`MCPConfig.ExportDBReadonly` ↔ JSON `export_db_readonly`
- 策略：`mcpexport` 读 `c.MCP.ExportDBReadonly` → `PolicyOpts.DBReadonlyConnector`
- UI：`api.MCPConfig` 增加可选 `export_db_readonly?: boolean`；`false` 可不序列化（omitempty）

## 5. 实现要点

| 文件 | 改动 |
|------|------|
| `web/chat/src/api.ts` | `MCPConfig.export_db_readonly?` |
| `web/chat/src/pages/connectorForms/mcp.ts` | `McpFormValues.exportDbReadonly`；`validateMcp` / `connectorToMcpForm` |
| `web/chat/src/components/settings/ConnectorEditorModal.tsx` | 复选框接线 |
| `web/chat/src/strings.ts` | 标签 + hint |
| 对应 `*.test.ts(x)` | 提交体含字段；回显 |
| `README.md` / `README.zh-CN.md` | MCP 导出节补一句 |
| ledger / 确认清单 / P3-C §11 | 交叉引用已由本史诗承接 |

## 6. 测试 / DoD

- `validateMcp` 勾选 → `mcp.export_db_readonly === true`；未勾选 → 字段缺省或 false
- 编辑弹窗：勾选后 `onSaveInfo` 携带该字段；加载已有连接回显勾选
- `npm test` 相关文件绿；`npm run build` 更新 `internal/ui/dist`
- 无新增 Go 行为要求（可选：既有 `policy_test` 仍绿）

## 7. 风险

| 风险 | 缓解 |
|------|------|
| 用户误以为影响本机对话/Run | hint 写明「仅导出」 |
| 默认 false 导致库 MCP 误导出写工具面 | 写类仍有 `IsMCPWriteTool` 硬拒绝；hint 建议库类开启 |
