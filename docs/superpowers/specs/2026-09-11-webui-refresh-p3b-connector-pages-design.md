# P3-B 业务系统 / 插件设置页人话化设计（连接器外壳）

> 状态：待批准
> 日期：2026-09-11
> 范围：WebUI 设置 →「业务系统（OpenAPI）」与「插件（HTTP 插件）」两页改版
> 前置：P3-A 表单原语（PageHeader / Card / Field / Input / Select / Modal / ConfirmDialog / Toast / EmptyState / Badge）已落地
> 备注：项目处于开发阶段，不保留历史数据兼容、不背技术包袱；目标是干净整洁

---

## 1. 背景与产品定位

本批针对「业务系统（OpenAPI）」与「插件（HTTP 插件）」两页。要点：

- 业务系统页与插件页后端是同一种 `Connector`（`type` 分别为 `openapi` / `http`），前端约 90% 代码重复，均为「列表 + 自绘抽屉」。
- 两页都没有使用 P3-A 原语，仍在用老式 `settings-*` 类、`window.confirm`，并把后端错误码 `code: message` 直接显示给用户。
- 页面裸露大量工程术语：`Connector`、`base_url`、`auth.mode`、`static / passthrough / vault_ref`、`import_format`、`execution_callback_url`、`require_login`、`require_approval` 等。

### 1.1 角色定位（关键产品判断）

- **管理员**负责「开通 / 限制白泽有哪些能力」：接入哪些业务系统、哪些工具默认需要本人登录 / 人工审批。
- **真正日常使用者是运营人员**，他们在被对接系统里各有自己的账号与数据权限。
- 因此助手调用业务接口时，应当**谁在用就用谁本人登录授权的身份**；不应由管理员配置一把全员共用的服务账号密钥，把各系统自带的人员权限体系抹平。

后端调用链已支持该模型（见 `internal/connector/register_one.go:126-167`）：对话内调用按会话身份解析；标记为 `require_login` 的工具在该会话未登录时返回 `login_required`，不会用服务端默认头顶上。

### 1.2 鉴权区决策

- **接入表单中彻底移除「服务端默认凭证」编辑**（`static / passthrough / vault_ref` 及其 headers、透传头、密钥库引用）。表单不再出现任何 token / 鉴权字段。
- 不展示、不回传历史默认凭证：连接器页保存即按空凭证提交（后端空 mode 规范化为空 static），旧的开发期凭证会被清空，这是预期行为。
- `execution_callback_url`（执行回调，高级可空字段）一并从界面移除，提交为空、清空，不保留隐藏状态。
- 个人鉴权只走「运营本人登录」路径；登录工具如何捕获 token 的 `capture` 配置仍归「助手功能 / Tools」页维护，不在本页出现。
- 「对话中 @ 一下即可登录某系统」的**登录入口直达**（确定性直达调用、@ 弹窗特殊条目、`login_required` 卡片快捷登录）是独立功能线，**不在本批**，见 §8 后继项。

---

## 2. 范围

### 做

- 新增公共「连接器外壳」组件（列表 / 空态 / 编辑 Modal），业务系统页与插件页共用。
- 两页改为基于 P3-A 原语的薄页面；接入 / 编辑改为两步 Modal（连接信息 → 工具权限勾选）。
- 术语中文化、错误码中文化（集中到 `strings.ts`）。
- 删除两页全部默认凭证 UI / state / 校验、旧抽屉、`code: message` 直出、`window.confirm`。
- 后端小改：PUT 连接器时把「默认凭证」与「登录捕获 capture」作为两个独立关注点，请求未带 capture 时保留原 capture（避免连接器页清空 Tools 页配置的登录捕获规则）。

### 不做

- 不改连接器 / 工具的数据模型与注册语义（除上述 capture 省略保留外）。
- 不做 MCP、MCP 导出、消息回调、外部来信页（后续批次）。
- 不做登录入口 @ 直达、`login_required` 前端卡片改造。
- 不为旧凭证、旧字段、旧跨页导出函数保留兼容别名。
- 不做运营只读视图（两页仍为 AdminOnly）。

---

## 3. 公共连接器外壳与页面交互

### 3.1 组件划分

- `web/chat/src/components/settings/ConnectorShell.tsx`
  - 展示型外壳：`PageHeader`（标题 + 一句话说明）+ 连接卡列表 + `EmptyState`（空态与主按钮）。
  - 接收：标题文案、说明文案、空态文案、主按钮文案、连接行数据数组、行渲染差异（业务系统 / 插件）、以及打开编辑 / 删除的回调。
- `web/chat/src/components/settings/ConnectorEditorModal.tsx`
  - 基于 `Modal` 的两步编辑器，替代自绘 `settings-drawer`。
  - 第 1 步连接信息表单由各页面以 `children` / 渲染槽注入差异（业务系统含文档导入区，插件仅服务地址）。
  - 第 2 步工具权限勾选由外壳统一渲染（数据来自保存响应的工具列表与连接器上的权限列表）。
- 数据加载仍在各页面：业务系统靠接口文档解析、插件靠 healthz，发现来源不同；外壳不耦合具体 API。

### 3.2 连接卡列表（两页统一）

每张连接卡展示：

- 连接名（ID）与服务地址；
- `N 个工具` 徽标；
- 一行权限摘要：如「3 个工具需本人登录 · 1 个需审批」，两者皆空则不显示该行；
- 右上角「⋯」菜单（`DropdownMenu`）：编辑、删除；
- 「去助手功能查看工具」链接保留，文案人话化。

不再显示 `static / passthrough / vault_ref / capture` 等技术徽章，不显示任何凭证信息。

空态文案：

- 业务系统：「还没有接入业务系统。上传一份接口文档，助手就能调用订单、工单等系统。」
- 插件：「还没有接入插件服务。接入你们自行部署、按白泽约定提供能力的程序后，助手即可使用其能力。」

### 3.3 接入 / 编辑 Modal（两步）

第一步 · 连接信息：

- 连接编号（ID）：新建可填，编辑锁定；提示「仅用于区分，保存后不可改，用小写字母 / 数字 / `-` / `_`」。
- 服务地址 base_url：占位符业务系统 `https://api.example.com`，插件 `http://127.0.0.1:19090`。
- 业务系统独有：接口文档「上传文件（.json/.yaml/.yml）」或「填文档链接」二选一；文档格式默认「自动识别」，可选手动项 OpenAPI 3 / Swagger 2 / Postman 合集。
- 插件无文档区。
- 不出现鉴权字段、不出现 execution_callback_url。

第二步 · 工具权限（保存成功后进入，Modal 不关闭）：

- `putConnector` 响应直接携带本次识别出的工具列表，因此无需额外请求即可渲染勾选。
- 每个工具一行，两个勾选框：「使用前需本人登录」「使用前需人工审批」，默认均不勾（公开工具）。
- 顶部说明：「勾选『需本人登录』后，每位运营用自己的账号访问，权限互不混用；不勾则任何人都能直接调用此工具。」
- 编辑已有连接时，进入 Modal 即可在本步用连接器已存的 `require_login` / `require_approval` 列表回显勾选。
- 新建时可只填第一步直接保存（权限默认全公开，第二步可跳过；之后随时编辑补勾）。

第二步勾选的保存：勾选状态映射为工具名列表，再次调用 `putConnector` 提交 `require_login` / `require_approval`；此次不传文档，后端在 spec 缺省时复用已保存的 spec（现有 openapi spec 缺省分支已支持），不触发重新抓取。

两步在 Modal 内可前后切换，默认停在第一步；无论新建还是编辑，第二步都可随时进入。编辑已有连接时，工具列表来自已加载的连接器详情（含 `tools` 与连接器上的 `require_login` / `require_approval`），第二步直接回显勾选，无需再次保存即可看到现状。

### 3.4 删除

- 使用 `ConfirmDialog` 替代 `window.confirm` 与自写删除确认。
- 文案讲清后果：「删除后，该连接提供的工具会从助手能力中移除。」

### 3.5 错误反馈

- 保存 / 删除 / 文档抓取等错误统一经 `ApiError` 转中文，以 `Toast` 呈现，不再拼接 `${code}: ${message}`。

---

## 4. 术语人话化对照

| 现状 | 改为 |
|---|---|
| 页面标题 `OpenAPI`；「添加 OpenAPI Connector」 | 标题「业务系统」；按钮「接入业务系统」；弹窗「接入 / 编辑业务系统」 |
| 标题「插件」；「HTTP 插件 Connector / 侧车 sidecar / 协议 v0 / healthz」 | 标题「插件服务」；说明「接入你们自行部署、按白泽约定提供能力的程序」；删除 sidecar / healthz / `/v0/tools` / invoke / 协议版本等措辞 |
| `Connector ID` | 「连接编号」+ 小写字母 / 数字 / `-` / `_` 提示 |
| `base_url` | 「服务地址」 |
| `import_format`（OpenAPI3/Swagger2/Postman） | 「接口文档格式」：自动识别（默认）/ OpenAPI 3 / Swagger 2 / Postman 合集 |
| 文档上传 / URL | 「接口文档」：上传文件或填文档链接；去掉 Swagger UI / host 措辞 |
| `require_login`（手填工具名） | 「使用前需本人登录」，改为工具勾选 |
| `require_approval`（手填工具名） | 「使用前需人工审批」，改为工具勾选 |
| `auth.static/passthrough/vault_ref` 徽章与表单 | 全部移除，界面不出现凭证概念 |
| `execution_callback_url` | 从界面移除，提交清空 |
| 「尚未注册 X Connector」「添加 Connector」 | 空态「还没有接入…」/ 按钮「接入…」 |

---

## 5. 错误码中文化

集中到 `strings.ts` 的错误码映射（复用 P1 的 `ApiError`）。本批覆盖：

| 后端 code / 情形 | 中文文案 |
|---|---|
| `invalid_spec` | 接口文档无法解析，请确认是有效的 OpenAPI / Swagger / Postman 文档 |
| `invalid_spec_url` | 文档链接格式不正确，请检查链接 |
| `spec_fetch_blocked` | 服务器不允许抓取该地址的文档（地址被安全策略拦截） |
| `spec_fetch_failed` | 无法下载该文档链接，请确认地址可以访问 |
| `unsupported_import_format` | 不支持的文档格式 |
| `invalid_plugin` | 无法从该插件地址识别到可用能力，请检查服务是否正常 |
| `tool_conflict` | 有工具与其它连接重名，请调整对方系统里的操作名称后重试 |
| `invalid_auth` | 连接保存的凭证无效，请联系管理员通过配置处理 |
| `base_url is required` | 请填写服务地址 |
| `spec is required` | 请提供接口文档 |
| 其它 / 未知 | 操作失败，请重试；仍失败请联系管理员（技术详情可折叠） |

前端表单本地校验：连接编号必填且符合 `^[a-z][a-z0-9_-]{0,63}$`；服务地址必填且为 http(s) URL；业务系统必须提供文档（文件或链接二选一）。校验失败在对应字段下以内联错误呈现，不弹后端式错误。
---

## 6. 后端改动：capture 的省略保留语义

### 6.1 问题

`PUT /v0/connectors/{id}` 目前用请求体整包构造 `store.ConnectorAuth`（`internal/api/server.go:848-867`），其中包含 `Capture`。连接器页改版后不再提交任何 `auth`，若直接按零值写入，会把「助手功能 / Tools」页配置的登录捕获规则清空，破坏本人登录链路。

默认凭证（static / passthrough / vault_ref）应当随连接器页提交被清空（开发期预期行为）；但 `capture` 是另一个关注点，必须与凭证解耦。

### 6.2 方案

在 PUT handler 增加 capture 的「省略 vs 显式」区分，与现有 `require_login` 用 `*[]string` 区分省略 / 空数组同一思路：

- 将 `authBody.Capture` 改为指针（或在 `authBody` 增加 `CaptureSet bool`，以能区分「未提供」与「显式空对象」为准）。
- 解析后、调用 `connector.Apply` 前：
  - 请求未提供 `auth.capture`：读取现有连接器 `Store.GetConnector(id)`，把其 `Auth.Capture` 原样带到本次 `connectorAuth.Capture`；连接器不存在（新建）则不设置，交给 `CaptureDefaults` 补默认。
  - 请求显式提供 `auth.capture`（含 `__none__` 关闭、空对象）：按提交值处理，维持现状语义。
- 凭证三字段（static / passthrough / vault_ref）不做保留，按请求体（连接器页为空）写入。
- Tools 页保存 capture 的路径显式提交 capture，行为不变。

该合并放在 API handler（已有 `GetConnector` 先例，见 openapi spec 缺省分支），`connector.Apply` 保持纯粹的注册入口、不引入隐式状态读取。

### 6.3 测试（Go）

- PUT 连接器不带 `auth.capture`、原有 capture 非默认：保存后 capture 保留。
- PUT 显式带 `auth.capture`（含 `tool_name_glob: "__none__"`）：按提交覆盖。
- 新建连接器（无现有记录）不带 capture：生效 `CaptureDefaults`（`*login*` 等）。
- 连接器页式提交（无 auth）：static / passthrough / vault_ref 为空，但既有 capture 仍在。

---

## 7. 前端文件结构与清理

新增：

- `web/chat/src/components/settings/ConnectorShell.tsx`：页头 + 连接卡列表 + 空态 + 行「⋯」菜单。
- `web/chat/src/components/settings/ConnectorEditorModal.tsx`：两步 Modal 与工具权限勾选。
- `web/chat/src/pages/connectorForms/types.ts`：表单态、校验结果类型。
- `web/chat/src/pages/connectorForms/validate.ts`：纯函数校验（连接编号正则、服务地址 URL、文档二选一）。

改写：

- `OpenApiSettings.tsx`、`PluginSettings.tsx`：瘦身为「数据加载 + 组合外壳 / Modal + 提交」。
- `strings.ts`：新增两页标题 / 字段 / 空态 / 权限文案与错误码中文映射。

删除（不留兼容别名）：

- 两页内全部鉴权模式 UI / state / 校验；旧 `settings-drawer` 用法；`code: message` 直出；`window.confirm` / 自写删除流程对这两页的使用。
- 仅服务于旧鉴权表单、且无其它引用的导出函数（如 `buildConnectorAuth`、`PluginAuthMode` 等），同步更新唯一引用方 `OpenApiSettings.tsx` 与相关测试。
- `parseKeyValueLines / parseLineList` 等键值解析：连接器两页不再使用即从两页移除；MCP 页自己的副本留待批次 C 统一收敛，本批不动 MCP。

保留：

- `captureForm.ts` 中 Tools 页仍使用的 capture 相关函数（`mergeAuthWithCapture` 等）；只删除连接器页不再使用的凭证合并路径。

共享纯函数被移除前，先全局检索引用（已知仅 `OpenApiSettings.tsx` 与其测试引用 `PluginSettings` 导出）。

---

## 8. 测试、验收与后继项

前端（vitest + jsdom）：

- `validate.ts`：编号正则、服务地址必填与 URL 格式、业务系统文档二选一。
- 外壳：列表渲染、空态、权限摘要文案、「⋯」菜单编辑 / 删除。
- Modal：第一步校验；保存成功进入第二步；第二步用响应工具渲染勾选并按 `require_login/require_approval` 回显；跳过第二步即可完成；勾选映射为工具名列表提交。
- 错误码 → 中文映射；删除走 ConfirmDialog。
- 全量 `tsc`、`vitest`、`npm run build`（重建 dist）。

后端：`go test ./internal/api/... ./internal/connector/...`，覆盖 §6.3。

人工走查：

- 接入一个公开接口（无鉴权）跑通；保存后勾选「需本人登录 / 需审批」，重开 Modal 回显正确。
- 业务系统：错误文档、文档链接不可达、工具重名，分别出现中文提示而非原始 code。
- 插件：地址不通 / 无能力时中文提示。
- 保存连接器不清空 Tools 页配置的 capture（回归验证 §6）。
- 暗色模式与移动端布局无溢出。

后继项（独立里程碑，本批不做）：

- 登录入口 @ 直达：后端从连接器 capture 配置实时推导登录入口、在 @ 弹窗作为特殊条目、绕过模型选择确定性直达登录工具；`login_required` 工具卡片显示「去登录」快捷入口。需改动 Run 工具调度与 @ 语义，单独立项设计。
- 批次 C：MCP 接入 / MCP 导出迁入连接器外壳。
- 批次 A：消息回调 / 外部来信人话化。