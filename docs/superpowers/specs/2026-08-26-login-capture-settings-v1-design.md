# 登录捕获设置 v1 设计规格

> 状态：已批准（2026-08-26）  
> 日期：2026-08-26  
> 前置：`2026-08-23-auth-capture-settings-v0`（OpenAPI Tools 页 capture 表单）、`2026-08-26-http-plugin-login-capture-v0`（HTTP 后端捕获）  
> 依据：头脑风暴确认「开发阶段一步到位」——实施方不靠 YAML/curl 即可配通 HTTP 侧车登录捕获全流程  
> 分支建议：`feat/login-capture-settings-v1`

---

## 1. 目标与成功标准

**目标：** 管理员在 **设置 → Tools** 为 `type: openapi` 与 `type: http` Connector 配置 `auth.capture`；在 **设置 → 插件 / OpenAPI** 编辑 Connector 时**不会意外重置**已在 Tools 页保存的捕获规则；列表可一眼看出 capture 状态。

**一句话成功标准：** 实施方仅通过 UI 完成「注册 HTTP 侧车 → Tools 配 capture → 勾选需要登录 → Chat 调 login → 账号页出现登录捕获 → 后续工具带 token」，且中途在插件页改 `base_url` 不丢 capture。

### 做

| 项 | 内容 |
|----|------|
| Tools 页 | `supportsLoginCapture` 含 `openapi` 与 `http`；复用现有五字段表单与同一「保存 Connector 设置」按钮 |
| 防丢配置 | `PluginSettings` / `OpenApiSettings` 的 `PUT` **merge 已有 `auth.capture`**（来自编辑前 GET 的 connector） |
| 列表摘要 | 插件 / OpenAPI 列表行显示 capture 状态文案 |
| 抽取 | `connectorSupportsLoginCapture`、`captureSummaryLabel`、可选 `CaptureSettingsFields` 组件 |
| 文案 | HTTP / OpenAPI 分类型 hint；插件页链到 Tools 页说明 capture 配置位置 |
| 测试 | `captureForm` / merge preserve 单测；PluginSettings 相关单测；`npm run build` |
| 清理 | 更新 `2026-08-23-auth-capture-settings-v0-design.md` 交叉引用；修正过时 API 测试断言 |

### 不做

- MCP capture UI（后端未启用捕获）
- 捕获规则在线试跑 / mock invoke 按钮
- 在 Plugin / OpenAPI 抽屉内 duplicate 完整 capture 表单（仍只在 Tools 编辑）
- 后端 PUT 部分 merge capture（由前端 merge 承担）
- Go 运行时捕获语义变更

### 可测成功标准

1. HTTP Connector 在 Tools 组头展示 capture 五字段；保存后 GET 回显（含服务端 `CaptureDefaults` 补全）
2. MCP Connector 组头不展示 capture
3. 在 Tools 为 HTTP 设 `tool_name_glob: custom_*` 后，在插件页仅改 `base_url` 保存 → GET 仍回显 `custom_*`
4. OpenAPI 同理：OpenAPI 页改 auth 不重置 Tools 已配 capture
5. 插件 / OpenAPI 列表行显示 capture 摘要（见 §3.3）
6. OpenAPI capture 与 HTTP 后端捕获回归仍绿；前端 build + 单测绿

---

## 2. 数据流与保存语义

### 2.1 capture 编辑入口（唯一）

**设置 → Tools** 展开 Connector 组头：与「执行回调 URL」同区，字段不变：

| 字段 | 控件 |
|------|------|
| `tool_name_glob` | 单行；`__none__` 关闭 |
| `token_json_paths` | 多行 |
| `label_json_paths` | 多行 |
| `header_template` | 单行 |
| `default_scheme` | 可选单行 |

保存：`putConnector` 带 `mergeAuthWithCapture(meta.auth, draft)` + `execution_callback_url` + 其余 connector 字段（与 v0 相同）。

### 2.2 插件 / OpenAPI 页保存（preserve capture）

`buildConnectorAuth` 仍只构建 mode / static / passthrough / vault_ref（不变）。

新增纯函数（建议放在 `captureForm.ts`）：

```ts
/** When saving from Plugin/OpenAPI settings, keep capture configured on Tools page. */
export function mergeAuthPreserveCapture(
  built: ConnectorAuth,
  existing: ConnectorAuth | undefined,
): ConnectorAuth
```

行为：若 `existing?.capture` 存在且至少一个字段非空（或曾显式 `__none__`），将 `capture` 复制到 `built`；否则不附加 capture（让服务端 `CaptureDefaults` 生效，与今日 omit 行为一致）。

`PluginSettings.onSubmit` / `OpenApiSettings.onSubmit`：在 `validate*` 成功后，`auth: mergeAuthPreserveCapture(validated.auth, loadedConnector.auth)`。

编辑时 `loadedConnector` 为 `openEdit` 时 GET 的 `ConnectorInfo`；需在 submit 时仍能访问（state 存 `editingAuthCapture` 或整份 `editingConnector`）。

### 2.3 类型支持矩阵

| Connector type | Tools capture UI | Plugin/OpenAPI 页 preserve | 列表摘要 |
|----------------|------------------|----------------------------|----------|
| `openapi` | 是 | OpenAPI 页 preserve | 是 |
| `http` | 是 | 插件页 preserve | 是 |
| `mcp` | 否 | N/A | 否 |

---

## 3. UI

### 3.1 共享纯函数与组件

**`captureForm.ts`（扩展）：**

- `connectorSupportsLoginCapture(type: string | undefined): boolean` → `type === 'openapi' || type === 'http'`
- `captureSummaryLabel(capture: ConnectorAuth['capture'] | undefined): string | null`  
  - 无 capture / 全空 → `null`（列表不显示）  
  - `tool_name_glob === '__none__'` → `捕获已关闭`  
  - 有 glob 且非空 → `捕获 ${glob}`（glob 过长可截断）  
  - 仅 paths/template 无 glob → `捕获（默认 *login*）`（少见，GET 通常已补全 glob）

**`CaptureSettingsFields.tsx`（新建，可选但推荐）：**

- Props：`connectorId`, `draft`, `onDraftChange`, `connectorType: 'openapi' | 'http'`
- 渲染五字段 + 类型相关 hint 一行
- `ToolsSettings` 引用该组件，减少 JSX 重复

### 3.2 Tools 页 hint

- **OpenAPI：** 「登录捕获：匹配 glob 的 operation 响应 JSON 写入会话身份。」
- **HTTP：** 「登录捕获：侧车中名称匹配 glob 的工具 invoke 成功后写入会话身份（如 `login`）。」
- 共用：「执行回调：invoke 走企业统一 URL。」

### 3.3 列表摘要

**PluginSettings** / **OpenApiSettings** 行内 `requireParts` 旁增加 capture 摘要（`captureSummaryLabel(info.auth?.capture)` 非 null 时显示）。

### 3.4 插件页导航说明

`PluginSettings` 页顶 `settings-meta` 增加一句：登录捕获与执行回调在 **Tools** 页按 Connector 配置（保留现有 Tools 链接）。

---

## 4. 测试计划

| # | 用例 | 位置 |
|---|------|------|
| 1 | `connectorSupportsLoginCapture('http'|'openapi'| 'mcp')` | `CaptureSettings.test.ts` |
| 2 | `captureSummaryLabel`：`__none__`、自定义 glob、空 | 同上 |
| 3 | `mergeAuthPreserveCapture`：有 capture 时保留；无 capture 时不附加 | 同上 |
| 4 | `validatePluginForm` + merge 形状（mock auth 输入） | `PluginSettings.test.ts` |
| 5 | 手工：Tools 配 HTTP capture → 插件页改 base_url → GET 仍保留 | 实现计划验收 |
| 6 | 修正 `TestPutHTTPPluginUsesIdentitiesNoCapture`：重命名并改为断言 HTTP PUT **保留** capture（或拆为 identity 消费与 capture 持久化两个用例） | `internal/api/server_test.go` |

前端：不强制 E2E；`npm run test` + `npm run build`。

---

## 5. 文档

- 本规格取代「仅 OpenAPI 展示 capture」的 v0 成功标准第 1 条；在 `2026-08-23-auth-capture-settings-v0-design.md` 文首增加「HTTP 与 preserve 见 `2026-08-26-login-capture-settings-v1-design.md`」
- README 中英已写 OpenAPI/HTTP capture；实现后确认与 UI 一致，无需大改

---

## 6. 非目标回顾

本里程碑补齐 **登录捕获的管理面闭环**（展示 + 防丢 + 可观测），不扩展 MCP、不 duplicate 表单、不改捕获运行时语义。

---

## 7. 实现提示（非绑定）

- `ToolsSettings.tsx`：`supportsLoginCapture` 改为调用 `connectorSupportsLoginCapture(c?.type)`
- 编辑插件时保存 `originalConnector` ref 或在 form state 存 `auth.capture` 快照
- `mergeAuthPreserveCapture` 与 `mergeAuthWithCapture` 职责分离：前者 preserve-only，后者 Tools 页主动编辑
- API 测试旧名 `UsesIdentitiesNoCapture` 易误导；新名如 `TestPutHTTPPluginPreservesCaptureAndUsesIdentity`

---

*头脑风暴 §「登录捕获设置 v1」已批准；实现前若字段名微调，以本文语义为准并更新本文。*
