# P3-C：外部工具服务（MCP）迁入连接器外壳 + 对外提供能力页人话化

- 日期：2026-09-12
- 状态：已交付（2026-09-13 账本对齐）
- 归属：WebUI 体验改版 P3 逐页人话化，批次 C
- 前置：P3-A（表单原语）、P3-B（业务系统/插件服务连接器外壳）已合并

## 1. 背景与目标

P3-B 抽出了可复用的连接器列表外壳 `ConnectorShell` 与两步编辑器 `ConnectorEditorModal`，并把「业务系统（OpenAPI）」「插件服务（HTTP Plugin）」两页迁入。连接类设置页还剩两条线：

- 入向：接入外部 MCP 工具服务（现 `McpSettings.tsx`，476 行，列表 + 自绘抽屉）。
- 出向：把白泽能力以标准 MCP Server 对外暴露（现 `McpExportSettings.tsx`，576 行，端点 + 调用方身份 + 导出 Key）。

本批目标：

1. 把「外部工具服务」迁入 P3-B 的连接器外壳 / 两步编辑器，交互与另外两类连接一致，消除旧抽屉、手敲工具名、`window.confirm`、裸错误码。
2. 「对外提供能力」方向相反、数据模型不同（全局身份与 Key，与连接器无外键关系），保留独立页与全部功能，仅做全面人话化，不改任何后端 API。
3. 命名对齐 P2 总体设计术语表，路由 id 一律不变（书签不失效）。

## 2. 已确认的产品决策

1. 范围：MCP 接入迁入外壳；导出页保留独立、全面人话化；不把两者合并进同一外壳。
2. 鉴权遵循 MCP 规范与主流 agent 统一做法，分传输方式：
   - stdio（本地子进程）：不用 OAuth，凭证走环境变量 `env`（白泽已支持占位符 `${VAR}` / `env:VAR` / `file:`）。
   - Streamable HTTP（远程）：本批支持静态请求头 / API Key（`headers`，常见 `Authorization=Bearer ...`）；规范的 OAuth 2.1 交互登录（401 发现、PKCE、DCR、浏览器回调、令牌存储刷新）单独立项，本批不做，仅在需要鉴权失败时给人话提示。
3. 工具级权限：保留「使用前需人工审批」（`require_approval`），对齐 Claude Code / Cursor / VS Code 对 MCP 工具的执行前人工确认惯例；MCP 不显示对其无效的「需本人登录」（白泽 capture 仅支持 openapi/http，MCP 无后端支撑，且它不属于 MCP 标准）。
4. 字段录入沿用多行文本框（新增共享 `Textarea` 原语），保留 `KEY=VALUE` / 每行一个格式与保存时解析，不做结构化行编辑器。
5. 命名：`mcp` 页显示「外部工具服务」（描述保留「标准 MCP」）；`mcp-export` 页显示「对外提供能力」。
6. 导出页人话化深度：保留三段结构与全部功能，全面替换文案、`window.confirm`、裸提示，集中 strings；一次性 Key 明文弹窗保留并加安全提示。

## 3. 范围

在范围内：

- `ConnectorKind` 扩展 `'mcp'`；外壳 / 编辑器 / 校验 / 文案支持第三种类型。
- 新增共享 `Textarea` 原语；多行解析纯函数下沉共享。
- `McpSettings.tsx` 瘦身为薄页，复用外壳与两步编辑器。
- `McpExportSettings.tsx` 人话化（ConfirmDialog / Toast / Modal / strings / 人话错误）。
- `settingsNav.ts`、`settingsHomeBadges.ts` 展示改名。
- 增补 `invalid_mcp` 等人话错误映射。
- 全量测试（vitest + tsc），重建 `internal/ui/dist`。

明确不做（见第 11 节）：OAuth、MCP capture/需本人登录、`export_db_readonly` UI、后端会话泄漏修复、Webhook/Inbox 人话化、结构化键值行编辑器。

## 4. 信息架构与命名

子路由与内部 id 全部保持不变（`/settings/mcp`、`/settings/mcp-export`），仅改用户可见名称与描述：

| id（不变） | 现展示 | 新展示 | 分组 |
| --- | --- | --- | --- |
| `mcp` | MCP | 外部工具服务 | connect |
| `mcp-export` | MCP 导出 | 对外提供能力 | connect |

- 改 `web/chat/src/settingsNav.ts` 两条 `label` / `desc`：
  - mcp 描述：接入标准 MCP 工具服务（本地子进程或远程 HTTP），扩展助手可用能力。
  - mcp-export 描述：把助手的能力以标准 MCP 服务对外开放，供其他客户端调用。
- 两页仍为 `AdminOnly`（`main.tsx` 现状不变），导航 operator 维持 `locked`。
- `settingsHomeBadges.ts` 的 mcp / mcpExport 徽章计数逻辑不变，仅其文案随 strings 更新（如「N 个外部工具」「N 个出口」）。
- 页面内标题、空态、按钮使用新名称；「MCP」作为技术术语保留在副标题、字段提示与帮助文案中，降低理解门槛并便于对号入座生态资料。

## 5. 外部工具服务：列表外壳

复用 `components/settings/ConnectorShell.tsx`，扩展第三种 kind：

- `ConnectorShellProps.kind` 与 `ConnectorKind` 增加 `'mcp'`。
- 标题 / 空态 / 新增按钮文案按 kind 三分支取自 `strings.ts` 的 `CONNECTORS`（新增 mcp 系列键）。
- 行数据 `ConnectorRowData` 增加可选 `summary?: string`：
  - openapi/plugin 仍传 `baseUrl`，渲染服务地址。
  - mcp 不传 `baseUrl`，传 `summary`：stdio 显示 `本地程序 · <command>`（参数过长不展开）；http 显示 `远程服务 · <url>`。
  - 行徽标沿用工具数；行内权限摘要沿用 `permissionSummary(loginNames, approvalNames)`，对 mcp 调用时 `loginNames` 恒为空数组，因此只会出现审批计数，不出现登录计数。
- 行操作：「能做什么」链接（跳 `/settings/tools`）+ 下拉菜单（编辑 / 删除）；删除走外壳内置 `ConfirmDialog`（busy 时遮罩/Esc 不可关、失败保留弹窗并显示人话错误、可重试），删除旧 `window.confirm` 依赖。
- 列表加载沿用三页同构模式：`listTools()` → 按 `source==='mcp'` 收集 connector id（现有 `mcpConnectorIds`，下沉到共享位置）→ 逐个 `getConnector(id)`，用 GET 响应的 `mcp` 字段生成 `summary`，用 `require_approval` 生成审批名单。

## 6. 外部工具服务：两步编辑器

扩展 `components/settings/ConnectorEditorModal.tsx`，第三步类型 `mcp`。

### 6.1 第一步：连接信息

字段（按「连接方式」Select 条件渲染，传输方式只有 `stdio` 与 `http`，不提供已被后端禁用的 standalone SSE）：

- 连接编号：`Input`，沿用 `CONNECTOR_ID_RE`，编辑态禁用。
- 连接方式：`Select`，选项「本地子进程（stdio）」「远程服务（Streamable HTTP）」。
- stdio：
  - 启动命令 `command`：`Input`，必填，placeholder 如 `npx`。
  - 启动参数 `args`：共享 `Textarea`，「空格或每行一个」，解析沿用 `parseArgsText`。
  - 环境变量 `env`：共享 `Textarea`，每行 `KEY=VALUE`，解析沿用 `parseKeyValueLines`；字段提示「密钥建议使用 `${VAR}`、`env:VAR` 或 `file:路径` 占位符，不要直接写死」。
- http：
  - 服务地址 `url`：`Input`，必填 http(s)，placeholder `https://example.com/mcp`。
  - 请求头 `headers`：共享 `Textarea`，每行 `KEY=VALUE`，placeholder `Authorization=Bearer ${TOKEN}`，解析沿用 `parseKeyValueLines`。

第一步提交体（整表 PUT；此步不发 `require_approval`，即 nil 保留既有审批位）：

```json
{
  "type": "mcp",
  "mcp": { "transport": "stdio", "command": "npx", "args": ["-y", "x"], "env": { "K": "V" } }
}
```

或

```json
{ "type": "mcp", "mcp": { "transport": "http", "url": "https://...", "headers": { "Authorization": "Bearer ..." } } }
```

不带 `base_url`、`spec*`、`import_format`、`auth`、`execution_callback_url`；空 `env`/`headers` 时省略该键。

**关键约束（后端现状）**：`connector.Apply` 对 `type=mcp` 的每次 PUT 都实时重跑工具发现（stdio 实时拉起子进程，http 短连接发现，最长约 30s，见 `internal/connector/apply.go:161-205`）；若省略 `mcp`，会按 stdio + 空命令返回 `invalid_mcp`，没有 openapi 那种「复用既有 spec」分支。因此：

- 编辑器在第一步保存成功后必须在组件状态中保留「已提交的 mcp 连接配置」（transport 及各字段），第二步保存权限时把该配置原样随 `require_approval` 一起回传，禁止省略 `mcp`。
- 第一步保存按钮进入 busy（复用现有 `saving`），期间遮罩/Esc 不可关闭；发现失败显示人话错误并停留在第一步，可修改重试。发现成功后 PUT 响应里的 `tools` 作为第二步工具列表（与另外两类一致），并调用 `onSavedInfo(id, connection)` 把已提交的连接负载交给页面缓存，同时触发列表刷新与成功 Toast。
- 新建态第一步成功即已持久化连接器；第二步可「完成」或「跳过」。

### 6.2 第二步：工具权限（MCP 仅审批一列）

- 复用现有权限纯函数（`permissions.ts` 的 selection / toggle / toNameLists）与列表渲染。
- 对 `kind==='mcp'` **只渲染「使用前需人工审批」复选框一列**，不渲染「需本人登录」；选择状态中 login 恒为 false。
- 提交时 `require_approval` 始终为数组（空数组=整表清空，`*[]string` 非 nil 语义），修复旧 MCP 页「空名单被省略→无法清空审批」的缺陷；`require_login` 省略（nil=保留，MCP 工具本就无 login 位）。第二步 PUT 同时回传第一步保留的完整 `mcp` 配置。
- 回调签名（消除 P3-B 里 openapi/plugin 专用的 `baseUrl` 形参对第三种类型的歧义）：定义判别联合 `SavedConnection`（`{kind:'openapi',id,baseUrl,spec?,importFormat}` | `{kind:'plugin',id,baseUrl}` | `{kind:'mcp',id,mcp:MCPConfig}`）。第一步 `onSaveInfo(conn: SavedConnection)`；成功后 Modal 保留该负载并经 `onSavedInfo(conn)` 回传页面缓存。第二步 Modal 调 `onSavePermissions(id, loginNames, approvalNames)`，页面用缓存的连接负载 + 两个名单拼出完整 PUT 体（mcp 页带 `mcp` 且省略 `require_login`，openapi/plugin 维持现状字段）。为覆盖「编辑态直接进入工具权限」（未经第一步保存），Modal 在 open 时用 `initial` 初始化连接负载——为此 `ConnectorEditorInitial` 增加可选 `mcp?: MCPConfig`，页面 `openEdit` 对 mcp 传入 `c.mcp`，并在打开编辑时就缓存对应负载。P3-B 两页接线同步从缓存取 `baseUrl`，并有测试锁定。
- 编辑态：第一步提供「直接进入工具权限」次级按钮（沿用现有 `nextToPermissions`），用 GET 已有的工具与审批名单初始化选择。

### 6.3 校验纯函数

新增 `pages/connectorForms/mcp.ts`（纯函数，可单测）：

- `validateMcp(values)`：id 复用 ID 规则；stdio 校验 command 非空；http 校验 url 非空且匹配 http(s)；args/env/headers 在解析阶段校验，非法 `KEY=VALUE` 行返回字段级错误（内联到对应 Textarea），错误文案集中 strings。
- `mcpSummary(mcp: MCPConfig): string`：生成第 5 节行摘要。
- `toMcpConfig(form): MCPConfig`：把文本字段经 `parseArgsText` / `parseKeyValueLines` 转为提交结构（空 map 省略）。

`types.ts`：`ConnectorKind` 增 `'mcp'`；新增 MCP 第一步表单值与字段错误类型（`transport/command/args/env/url/headers`，错误键含 `command/url/args/env/headers`）。`validate.ts` 现有 openapi/plugin 逻辑不动，MCP 走独立模块，避免把条件分支塞进通用校验。

## 7. 共享原语与旧函数下沉

- 新增 `components/ui/Textarea.tsx` 并从 `components/ui/index.ts` 导出：受控、`disabled`、`invalid`（错误态边框）、与 `Input` 同一套令牌样式（`--space` / `--border` / 暗色），`rows` 可配。供 MCP 表单与后续批次复用；不引第三方组件库。
- 把多行解析纯函数从 `pages/McpSettings.tsx` 下沉到 `pages/connectorForms/lines.ts`：
  - `parseKeyValueLines`、`formatKeyValueMap`、`parseArgsText`、`parseLineList`（签名与现有行为保持不变，错误文案集中 strings）。
- 更新引用方改从共享模块导入：`McpExportSettings.tsx`、`InboxSettings.tsx`、`WebhookSettings.tsx`（解除它们对旧 MCP 页面文件的导入耦合）；迁移相关纯函数测试到 `lines.test.ts`，保证行为不回归。
- `mcpConnectorIds`（source `mcp` 过滤、去重、保序）一并放入共享位置（与 openapi/plugin 的 id 收集器并列），旧 `McpSettings.test.tsx` 中的纯函数测试迁移到对应新测试文件。
- 删除在三页全部迁移后已无引用的 `pages/connectorDelete.ts` 及其测试（旧 `window.confirm` 删除依赖）；删除旧抽屉标记与无 CSS 规则的 `settings-mcp*` 类。`.settings-drawer*` 样式因导出页 token 弹窗迁到 `Modal` 后可能仍被别处引用，删除前需 grep 确认无引用再清理（见第 8 节）。

## 8. 对外提供能力页人话化（独立页，不改 API）

页面仍为三段单页，路由 `/settings/mcp-export` 与所有 `/v0/settings/mcp-export*` 请求保持不变：

1. 端点：只读地址 + 复制按钮 + 静态接入示例；术语改人话（如「外部客户端接入地址」），enabled 状态用 `Badge` 展示。
2. 调用方身份（identity）：列表 + 行内编辑 + 新建；`scheme`/`headers` 给中文标签与帮助说明（请求头每行 `KEY=VALUE`，复用 `lines.ts`）。
3. 导出 Key：列表 + 新建 + 撤销；创建成功后的一次性明文 token 改用 `components/ui/Modal`（替换自绘抽屉），含醒目标题「密钥仅显示这一次」、复制按钮与「我已保存」关闭，不再提供二次查看明文的入口（后端本就只存哈希）。

交互与文案：

- 两处删除/撤销（身份、Key）的 `window.confirm` 全部换 `ConfirmDialog`（危险态、busy 防误关、失败保留可重试）。
- 成功/失败裸 `<p>` 文本统一换 `Toast`（成功）与人话内联错误（失败）；新增 strings 键覆盖端点、身份、Key 三组标题/字段/按钮/确认/错误，页面不再硬编码中文。
- 保留页头关于「鉴权使用专用导出 Key（非 Gate）」「工具是否导出在『能做什么』页按工具配置」的说明，改用人话表达；不新增工具级 export 策略编辑（仍在 Tools 页）。

## 9. 错误人话化

- `CONNECTOR_CODE_TITLES` 增补 `invalid_mcp`：标题如「无法连接到这个外部工具服务」，`detail` 结合后端 message 细分（命令不存在 / 占位符无法解析 → 检查启动命令与环境变量；HTTP 连不上 → 检查地址与网络；0 工具 → 服务未提供任何工具；401/403 或鉴权失败 → 提示补充请求头中的 API Key，OAuth 交互登录暂不支持）。
- `tool_conflict`、加载失败等沿用 P3-B 既有映射；MCP 列表/保存/删除统一走 `connectorErrorText`，不再出现裸 `invalid_mcp: ...`。
- 导出页身份/Key 接口的错误给同类人话映射（名称冲突、Key 不存在、字段非法等按 code/message 映射到 strings）。

## 10. 测试策略（TDD）

纯函数先行（红→绿），再组件，再薄页接线，导出页交互补齐：

- `connectorForms/lines.test.ts`：迁移 `parseKeyValueLines` / `formatKeyValueMap` / `parseArgsText` / `parseLineList` 正反例（行为与现测试一致）。
- `connectorForms/mcp.test.ts`：id 规则；stdio command 必填；http url 必填与 http(s) 校验；非法 `KEY=VALUE` 行字段级报错；空 env/headers 省略；`toMcpConfig` 两种 transport 输出形状；`mcpSummary` 两种摘要。
- `components/ui/Textarea.test.tsx`：受控值、disabled、invalid 态透传。
- `ConnectorShell.test.tsx`：kind=mcp 标题/空态/按钮文案；行渲染 `summary`；权限摘要只出现审批计数。
- `ConnectorEditorModal.test.tsx`（MCP 用例）：transport 切换显隐字段；stdio/http 两种第一步提交体形状（含不带 base_url/spec/auth）；第二步提交回传完整 `mcp` 配置 + `require_approval` 数组且无 `require_login` 键；空数组清空；编辑态「直接进入工具权限」回显名单；发现失败停留第一步并显示人话错误；busy 防误关；文件类无关逻辑不出现。
- `McpSettings.test.tsx`：薄页加载（listTools→mcp id→getConnector）、新建两步 PUT 次数与请求体、编辑回显、删除走 ConfirmDialog 调 `deleteConnector`；`mcpConnectorIds` 过滤/去重/保序。
- `McpExportSettings.test.tsx`：身份与 Key 删除走 ConfirmDialog（替换 window.confirm）；成功 Toast；一次性 Key 弹窗的安全文案与复制；人话错误渲染。
- 回归：`npm run test`（vitest）全绿、`tsc --noEmit` 无错；本批无 Go 逻辑改动，仍跑一次 `go test ./...` 兜底；最后 `npm run build` 重建并提交 `internal/ui/dist`。

## 11. 明确不做

- 远程 HTTP MCP 的 OAuth 2.1 交互登录（401 + PRM 发现、PKCE、DCR、浏览器回调、令牌存储与刷新、按连接器/按用户身份）：独立工作流单独立项；本批仅静态请求头与鉴权失败人话提示。
- MCP 工具的「需本人登录」与 capture：白泽 capture 仅支持 openapi/http，MCP 无后端链路，且非 MCP 标准，UI 不暴露。
- `export_db_readonly` 的任何 UI 写入（当前仅 YAML/bootstrap 可设）。
- 后端既有问题：DELETE 连接器不关闭 stdio 池会话的潜在泄漏；本批不改后端，仅在计划/风险中记录。
- Webhook（出站事件推送）/ Inbox（外部来信）页人话化（连接批次 A）。
- args/env/headers 的结构化行编辑器与密钥密码框遮盖（本批用 Textarea + 占位符提示）。
- 任何后端路由、字段名、错误码、角色语义变更。

## 12. 后端契约（只读引用，不修改）

- `PUT/GET/DELETE /v0/connectors/{id}`；`type=mcp`；`mcp: {transport,command,args,env,url,headers}`（`internal/store` 的 `MCPConfig`）。
- PUT 对 mcp 每次实时发现：stdio 需 `command`，http 需 `url`；错误码 `invalid_mcp`（命令/占位符/连接/0 工具）、`tool_conflict`(409)；发现最长约 30s。
- `require_approval` / `require_login` 为 `*[]string`：nil=保留 catalog 既有位，非 nil（含空切片）=整表重写。MCP 跳过 auth 处理。
- GET/PUT 连接器响应含 `mcp`、`tools`、`require_approval`、`require_login`（空名单序列化为 `[]`，P3-B 已保证）。
- 导出页：`GET /v0/settings/mcp-export`、`GET/POST …/identities`、`PATCH/DELETE …/identities/{id}`、`GET/POST …/keys`、`DELETE …/keys/{id}`；Key 明文仅创建时返回一次。
- 工具级导出策略 `export`(default/force_allow/force_deny) 仍经 `PATCH /v0/tools/{name}` 在「能做什么」页配置，本页不碰。

## 13. 主要改动文件

新增：

- `web/chat/src/components/ui/Textarea.tsx`
- `web/chat/src/pages/connectorForms/lines.ts`、`mcp.ts`
- 对应 `*.test.ts(x)`

修改：

- `web/chat/src/pages/connectorForms/types.ts`（kind + MCP 表单/错误类型）
- `web/chat/src/components/ui/index.ts`（导出 Textarea）
- `web/chat/src/components/settings/ConnectorShell.tsx`（mcp 文案、`summary`）
- `web/chat/src/components/settings/ConnectorEditorModal.tsx`（MCP 第一步条件字段、保留并回传 mcp 配置、第二步仅审批列）
- `web/chat/src/pages/McpSettings.tsx`（瘦身为薄页）
- `web/chat/src/pages/McpExportSettings.tsx`（人话化、ConfirmDialog/Toast/Modal/strings）
- `web/chat/src/settingsNav.ts`、`web/chat/src/settingsHomeBadges.ts`（展示名/徽章文案）
- `web/chat/src/strings.ts`（mcp 连接键、导出页键、`invalid_mcp` 错误映射）
- `web/chat/src/styles/components.css`（Textarea、MCP 表单间距等；只许用令牌）
- `web/chat/src/pages/InboxSettings.tsx`、`WebhookSettings.tsx`（改引共享 `lines.ts`）
- `internal/ui/dist/**`（重建产物）

删除：

- `web/chat/src/pages/connectorDelete.ts` 及其测试（确认无引用后）
- 旧 `McpSettings` 内抽屉标记与失效 `settings-mcp*` 类；无引用的 `.settings-drawer*` 规则（grep 确认后）

## 14. 风险与对策

- 第二步 PUT 误清空连接：后端对 mcp 无「省略即复用」，必须在编辑器状态保留并回传完整 mcp 配置；以请求体断言测试锁死。
- 发现耗时最长约 30s：按钮 busy + 防误关，文案表明正在连接；本批不加前端超时（后端已有 30s 上限），避免重复计时。
- 函数下沉牵动 Inbox/Webhook/McpExport：先下沉再改引用，迁移现有纯函数测试保证零行为变化。
- `.settings-drawer*` 可能仍被导出页 token 弹窗等使用：迁 Modal 后 grep 确认引用归零再删样式，否则保留。
- 删除共享 `connectorDelete.ts` 前确认三页及测试均无引用。
- 暗色/移动端：新增 Textarea 与条件字段只用令牌，纳入 P4 走查；长工具列表的 Modal 内滚已在 P3-B 解决。
