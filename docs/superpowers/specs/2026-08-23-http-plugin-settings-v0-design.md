# HTTP 插件设置页 v0 设计规格

> 状态：待审查  
> 日期：2026-08-23  
> 前置：HTTP 插件协议 v0 已落地；MCP 设置页 `McpSettings` 已落地  
> 依据：`docs/architecture-and-plugin-protocol.md` §4.2；Chat UI 壳 `/settings/plugins` 仍为 `ComingSoon`

---

## 1. 目标与成功标准

**目标：** 管理员在 `/ui/settings/plugins` 通过表单注册/编辑 `type: http` 侧车 Connector，无需手写 `curl PUT`。

**成功标准：**

1. 列表展示已注册 HTTP 插件 Connector（从 `GET /v0/tools` 过滤 `source=plugin` 聚合 `connector_id`，`GET /v0/connectors/{id}` 拉详情）
2. 表单：`id`、`base_url`、`auth.mode`（`static` / `passthrough` / `vault_ref`）、对应 headers 字段、`require_approval`（每行工具名）、`require_login`（每行工具名，可选）
3. 保存 `PUT /v0/connectors/{id}`，`type: http`；错误展示 `error.code`（`invalid_plugin`、`tool_conflict`、`invalid_auth` 等）
4. 链到 `/settings/tools`；说明指向 `examples/http-plugin` 与 README
5. `npm test` + `npm run build`；提交 `internal/ui/dist/**`

**不做：**

- OpenAPI Connector 注册（仍走 Tools 页手加 / YAML）
- `auth.mode: capture` 完整表单（v0 仅 static/passthrough/vault_ref；capture 仍 YAML/API）
- 侧车进程启停管理（文档说明用户自启 `examples/http-plugin`）
- 新后端 API（复用现有 PUT/GET connector）

---

## 2. UI 行为

对称 `McpSettings.tsx`：

| 区域 | 内容 |
|------|------|
| 说明 | HTTP 插件协议 v0；侧车须实现 healthz + `/v0/tools` + invoke |
| 列表 | connector_id、base_url、auth.mode、工具数、require 名单摘要 |
| 抽屉表单 | 新建/编辑；校验 id、base_url 非空 |
| auth.static | headers 多行 `Key=Value`（支持 `${VAR}` / `env:` 引用形状，不展开明文） |
| auth.passthrough | 每行一个 header 名 |
| auth.vault_ref | headers 多行 `Key=vault:...` 或文档约定引用 |
| 保存 | `putConnector` 扩展 body；成功后刷新列表 |

---

## 3. API / 类型

扩展 `web/chat/src/api.ts`：

- `ConnectorInfo`：`base_url`、`auth`（与 PUT 响应一致）、`spec` 可选
- `putConnector` body：`type: 'http'`、`base_url`、`auth`、`require_approval?`、`require_login?`

GET 回显 auth 引用形状（与 MCP env 一致，不展开密钥）。

---

## 4. 测试

- `PluginSettings.test.ts`：纯函数（connector id 聚合、headers 解析/格式化），对齐 `McpSettings.test.ts`
- 手动：`baize demo` + 起 `examples/http-plugin` → 设置页保存 → Tools 见 plugin 工具

---

## 5. 文档

README.zh-CN / README：设置 → 插件 可图形配置 HTTP 侧车（替换「仍通过 PUT 手动注册」表述）。

---

*批准后进入 `writing-plans` 实现计划。*
