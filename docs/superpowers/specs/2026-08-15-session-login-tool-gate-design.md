# Baize 设计规格：按会话登录、工具「需要登录」与 Connector 热更新接线

> **更新（2026-08-26）：** HTTP 插件「不捕获」边界已由 `2026-08-26-http-plugin-login-capture-v0-design.md` 取代；实现以新规格为准。

> 状态：已批准  
> 日期：2026-08-15  
> 前置：会话身份库、Connector 三种默认凭证模式、HTTP 插件 v0、Chat UI 壳已落地  
> 依据：架构草案 README 中「PUT `/v0/connectors` 未挂 Identities / Resolver / Capture」的缺口；本里程碑头脑风暴锁定的产品决策

本规格修订 `2026-08-13-connector-auth-modes-design.md` 的 invoke 优先级第 3 步（见 §7），不回退 `bearer_env`。

---

## 0. 动机

操作员已经能在对话里登录、把凭证记在该会话上，也能在设置里看到 Tools。但两条线还没对齐：

1. **热更新缺口。** `baize start` 会给 OpenAPI Connector 挂上会话身份库、选身份逻辑，以及登录结果捕获。`PUT /v0/connectors/{id}` 只更新默认凭证模式，不挂这三样。不重启进程就无法让 Identities / 登录捕获与启动路径一致。
2. **「人」和「机器」混用同一份进程级 Token。** 配置里的 `BAIZE_CONNECTOR_TOKEN` 会在对话没有捕获身份时静默顶上。聊天里看起来像「已经登录」，其实用的是部署时那把共享钥匙。人应该用自己的账号；那把钥匙只留给没有对话的脚本 / curl。
3. **「要不要登录」不能靠 OpenAPI `security` 猜。** 遗留规格经常缺字段或根本不是规范 OpenAPI。每个工具需要操作员可改的 **需要登录** 开关；解析进来默认当公开工具。

本里程碑先把「人」做对：对话内登录、公开工具免登录、需登录工具没账号就明确失败。工具的增删、MCP、侧车自己的登录捕获，都不在本轮。

---

## 1. 目标与成功标准

**目标：** 聊天按会话使用个人登录；未登录只能调公开工具；配置里的 Connector Token 改为可选，且带 `conversation_id` 的 Run 不得静默使用它。`PUT` Connector 与 `baize start` 共用同一套接线（身份库、选身份、OpenAPI 登录捕获）。每个工具有可改的「需要登录」，热替换 Connector 时不得无故清掉已设开关。

**成功标准：**

1. 对话里未登录：公开工具（「需要登录」关）可以调用；打开了「需要登录」的工具不发下游 HTTP，工具结果为错误（见 §9），Run 不进入 `waiting_human`
2. 同一 `conversation_id` 下先调登录类工具并捕获身份后，再调「需要登录」的工具，带上该会话凭证，不使用配置里的进程级 Token
3. 带 `conversation_id` 的 Run（Chat UI 始终带）即使配置了 `static` / `vault_ref` / `passthrough` 默认头，也**不得**在未捕获身份时用这些默认头顶上
4. 不带 `conversation_id` 的请求仍可使用三种 mode 的默认头（机器 / curl）；本开关不拦截这条路径
5. `configs/default.yaml` 开箱不再要求 `BAIZE_CONNECTOR_TOKEN`；未设置该变量时 `baize start` 仍能起来，Demo 只读路径可用
6. `PUT /v0/connectors/{id}` 之后，OpenAPI 登录捕获与身份选择与 `baize start` 一致，无需重启 Runtime
7. 设置 → Tools 可切换「需要登录」；`GET /v0/tools` 回显 `require_login`；PUT 未显式提交该列表时保留已设开关

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 接线 | 抽出 `connector.Apply`，`baize start` 与 `PUT /v0/connectors/{id}` 共用：解析默认凭证、挂 Identities / Resolver；OpenAPI 再挂 Capture（含与启动相同的缺省） |
| 对话 vs 机器 | 有 `conversation_id`：不用 Connector 默认头；无会话：三种 mode 行为与 2026-08-13 规格一致 |
| 开箱配置 | `default.yaml` 去掉对 Connector Token 的硬依赖；`static.headers` 可空 |
| 工具开关 | Registry / `tool.Info` 增加 `require_login`；YAML 可选列表；PATCH 单个工具；设置页开关 |
| 热替换 | PUT 省略 `require_login` 时保留仍存在的同名工具开关；显式数组则整表替换该 Connector 下的开关 |
| HTTP 插件 | PUT / start 挂 Identities 与 Resolver；本轮**不**从插件 invoke 结果做登录捕获 |
| 文档 | README 中英删除「PUT 未挂会话能力」的后续增强说明；架构草案 invoke 优先级与本规格 §7 对齐 |
| 测试 | 见 §10 |

### 不做

- MCP、Connector 可视化编辑器、工具增删 / 启停目录
- PUT 增加 `require_approval_mutating`（YAML 启动路径保持现有行为即可）
- HTTP 插件侧的登录捕获（插件自己的 `*login*` 结果本轮不写入身份库）
- YAML DSL、Webhook、SDK、Channel
- Runtime 直连业务数据库
- 企业回调 `callback_urls` / 架构草案 §4.3
- 把「需要登录」写入 SQLite（与现有 `require_approval` 一样：进程内 Store + YAML；重启后以 YAML / 再次 PUT 为准）
- 为 Chat UI 单独做登录引导页；未登录错误走现有工具卡片 `is_error` 展示
- 跨用户鉴权网关、多租户 IAM、OAuth / refresh / 401 自动换马

---

## 3. 产品行为

### 3.1 两种调用方

| 调用方 | 如何识别 | 凭证 |
|--------|----------|------|
| 人（Chat） | Run 带 `conversation_id` | 只用该会话已捕获（或强制 `identity_id`）的身份。没有身份时，公开工具空头调用；需登录工具失败。 |
| 机器（curl / 脚本） | Run **不带** `conversation_id` | 继续用 Connector 的 `static` / `passthrough` / `vault_ref` 默认头。不看「需要登录」。 |

Chat UI 创建 Run 必须继续带当前对话的 `conversation_id`，因此走「人」这一列。

### 3.2 「需要登录」

- 每个已注册工具一个布尔开关，JSON 字段 `require_login`，与 `require_approval` 相互独立。
- **解析注册时默认 `false`（公开）。** 不读取、不推断 OpenAPI `security` / `securitySchemes`。
- 操作员可在设置 → Tools 打开或关闭；也可在 YAML / PUT 里用名称列表声明。
- 登录类工具本身通常保持公开（否则无法先登录）。本规格不强制工具名，由操作员决定。

### 3.3 开箱 Token

- `configs/default.yaml`：**不要**再写 `Authorization: Bearer ${BAIZE_CONNECTOR_TOKEN}`。`auth.mode` 仍为 `static`，`static.headers` 可省略或为空。
- 若 YAML / PUT **显式**写了某个 header 且值为 `${ENV}`，而该环境变量未设置或展开为空：注册仍失败，`invalid_auth`（与 2026-08-13 一致）。想「没有默认钥匙」就不要写该 header。
- Docker / 集成测试若仍注入 `BAIZE_CONNECTOR_TOKEN`，只服务无会话的机器路径；不能让 Chat 演示依赖它。

---

## 4. 架构

启动与 PUT 不得各写一套注册逻辑。`internal/bootstrap` 已 import `internal/api`，共享代码不能放进 bootstrap。

新增 `internal/connector` 根包（`package connector`，与现有子包 `openapi` / `httpplugin` 并存）：

```
YAML（baize start）  ──┐
                      ├──► connector.Apply(ApplyInput)
PUT /v0/connectors    ──┘          │
                                   ├─ authcred.ResolveDefaults
                                   ├─ withCaptureDefaults（仅 OpenAPI）
                                   ├─ 合并 require_login（见 §6.3）
                                   ├─ openapi.RegisterWithOpts 或 httpplugin.RegisterWithOpts
                                   │     Identities + OpenAPISecurityResolver
                                   │     OpenAPI 另传 Capture；HTTP 插件不传 Capture
                                   └─ 更新进程内 AuthMode / AuthWhitelist（PUT 成功后，行为与现 handlePutConnector 相同）
```

`ApplyInput` 至少包含：Store、Registry、Identities、Connector id / type / spec / base_url、auth 配置形状、已解析或未解析的默认头（由 Apply 内调用 `ResolveDefaults`）、`require_approval`、可选 `require_login`（含「省略 vs 空数组」，见 §6.3）、YAML 专用的 `require_approval_mutating`（仅 start；PUT 本轮不传，视为 false）。

`withCaptureDefaults` 从 bootstrap 挪到 `connector` 包（或 Apply 内未导出函数），语义不变：

- 省略 `tool_name_glob` → `*login*`，并补 token / label 路径与 `Bearer {{token}}`
- `tool_name_glob: "__none__"` 关闭捕获

OpenAPI 的 `DefaultScheme` 推断（规格里唯一 scheme）保持启动路径现有行为，PUT 同样走 Apply，因此也会做。

HTTP 插件：`RegisterOpts` 继续无 Capture 字段。本轮不从插件结果写 Identity。同一对话里若已有 OpenAPI 登录捕获的身份，插件工具仍可通过 Resolver 用上（身份库是会话级，不是 Connector 级）。

---

## 5. PUT / GET Connector 与 Capture

### 5.1 `auth.capture`

`PUT /v0/connectors/{id}` 的 `auth` 增加可选 `capture`，形状与 YAML `connector.auth.capture` 相同：

```json
{
  "tool_name_glob": "*login*",
  "token_json_paths": ["accessToken", "data.accessToken", "data.token"],
  "label_json_paths": ["email", "data.email"],
  "header_template": "Bearer {{token}}"
}
```

- 省略 `capture` 或省略 `tool_name_glob`：与 `baize start` 相同的缺省（`*login*` 等）。
- `tool_name_glob` 为 `"__none__"`：关闭捕获。
- HTTP 插件类型：请求里即使带 `capture` 也**忽略**，不报错，GET 不回显插件 Capture（插件本轮无捕获）。

`store.ConnectorAuth` 增加 Capture 字段（配置形状，不含秘密）。进程内 Connector 映射，无需 SQLite 迁移。

### 5.2 回显

成功 PUT 与 GET `/v0/connectors/{id}`：

- 继续回显 `auth.mode` 与 `static` / `passthrough` / `vault_ref` 的**引用形状**，不回显展开后的秘密
- OpenAPI 回显 `auth.capture` 的配置形状（含缺省补全后的值，便于操作员看见实际生效的 glob）
- 回显 `require_approval` 与 `require_login` 名称列表（值为 true 的工具名，排序稳定，建议字典序）

### 5.3 Identities / Resolver

Apply 必须把进程的 `identity.Store` 与 `authresolve.OpenAPISecurityResolver{}` 传入 OpenAPI 与 HTTP 插件的 `RegisterOpts`。PUT 成功后，无需重启即可：登录工具结果写入该会话身份，后续工具按 §7 选身份。

---

## 6. 工具「需要登录」的配置面

### 6.1 Registry

`tool.entry` / `tool.Info` / `tool.Meta` 增加 `RequireLogin bool`，JSON：`require_login`。

`List()` 输出该字段。HITL 的 `require_approval` 不变。

注册时：名称出现在 `require_login` 列表中则为 true，否则 false。**禁止**用 HTTP 方法或 OpenAPI security 自动改这个值。

YAML：`connector.require_login` 为可选字符串数组，缺省空（全公开）。

### 6.2 `PATCH /v0/tools/{name}`

请求体：

```json
{ "require_login": true }
```

- 未知工具：`404`，`code` 为 `not_found`
- 非法 JSON / 缺少布尔字段：`400` `invalid_request`
- 成功：更新 Registry 该工具标志，并更新该工具所属 Connector 在 Store 中的 `require_login` 列表（加入或去掉该名称）；响应为更新后的 `tool.Info`
- 本轮 PATCH **只**改 `require_login`，不改 `require_approval`、不改描述

设置 → Tools：每条工具一行保留现有「需审批」徽章；增加「需要登录」开关，调用上述 PATCH，失败时页内报错，不静默。

### 6.3 PUT 时保留开关

对**正在被替换的那个 Connector**：

| PUT JSON | 行为 |
|----------|------|
| 省略 `require_login` 键 | 替换工具表之后，**同名仍在**的工具恢复 PUT 之前的 `require_login`；新出现的工具名为 `false`；已消失的名字丢掉 |
| `"require_login": []` | 该 Connector 下全部为 `false` |
| `"require_login": ["create_ticket"]` | 列出的为 true，该 Connector 其余为 false |

实现上 PUT body 必须能区分「省略」与「空数组」（例如 `*[]string` 或等价）。不能用 `len==0` 把两者当成一样。

`require_approval` 保持**现有** PUT 语义（调用方每次提交完整列表）；本规格不改审批列表的省略语义，以免行为突变。

启动路径没有「上一份开关」：只使用 YAML 列表。

---

## 7. Invoke 优先级（修订 2026-08-13 §3）

每次工具 invoke：

1. 强制 `identity_id`（若有且存在且未过期）
2. 会话内未过期、scheme 匹配的 Identity（无 `security` 时与现逻辑相同：活跃身份里按默认 / 最近使用挑选）
3. **仅当 Run 没有 `conversation_id`：** Connector 默认 Headers（`static` / `passthrough` / `vault_ref`）
4. 空头

然后：

- 若 Run **有** `conversation_id`，且该工具 `require_login == true`，且第 1–2 步没有得到可用凭证头：不发下游 HTTP，返回 §9 错误。登录门闸在 HITL 之前：此时不进入 `waiting_human`。
- 若 Run **没有** `conversation_id`：不应用 `require_login`；第 3 步默认头照常。

有会话时**禁止**把 Connector 默认头传进 Resolver（或等价地设置 SkipDefaultHeaders）。有身份时身份仍然优先，与 2026-08-13 一致。

`require_login` 与 `require_approval` 同时为 true：先通过登录门闸，再走现有 HITL。

---

## 8. HTTP 插件本轮边界

- start 与 PUT 都要挂 Identities + Resolver（今天 start 已挂，PUT 未挂）。
- 不把 Capture 传入插件注册；插件 invoke 成功结果不写入身份库。
- 插件工具同样有 `require_login`；若打开且当前会话没有身份（例如尚未用 OpenAPI 登录工具捕获），对话路径按 §7 失败。
- 插件自己的登录捕获列为后续里程碑，本规格不预留半套字段。

---

## 9. 错误

对话路径未登录且工具需要登录：

- 工具 `Invoker` 返回 `isError=true`，`err == nil`（与现有下游 4xx 工具错误同一档：记入轨迹，不把整个 Run 打成引擎崩溃）
- `content` 固定包含：
  - `code`: `"login_required"`
  - `message`: `"此工具需要先登录"`
- HTTP 控制面不为此单独增加新的 Run `error` code；操作员在工具卡片上看到错误即可
- 不得把凭证或默认 Token 写进 events / GET run / SSE

PUT / start 鉴权解析失败：仍为 `400 invalid_auth`，Registry / Store 不部分更新（现有保证）。

PATCH 未知工具：`404 not_found`。

---

## 10. 测试与验收

至少覆盖：

1. **Apply 共用：** PUT OpenAPI 后，同会话先 login 捕获，再调需登录工具，请求头为捕获凭证，不是配置里的 static Token；进程未重启。
2. **PUT 省略 capture：** 生效 glob 为 `*login*`（可用测试用登录工具名断言捕获发生）。
3. **PUT `tool_name_glob: "__none__"`：** 登录工具成功也不写入身份。
4. **对话不用默认头：** 配置了 static Token；带 `conversation_id`、无身份、公开工具：下游收不到该 Token。
5. **机器路径不变：** 无 `conversation_id` 时 static / 现有集成测试仍通过。
6. **需要登录：** 有会话、无身份、`require_login=true` → `login_required`，下游未被调用；登录后门闸通过。
7. **HITL 顺序：** `require_login` 且 `require_approval` 且未登录 → 不是 `waiting_human`。
8. **PATCH：** 开关翻转后 `GET /v0/tools` 与再次 invoke 行为一致。
9. **PUT 省略 `require_login`：** 热替换后同名工具开关仍在；新工具名为 false。
10. **PUT `"require_login": []`：** 该 Connector 下全部公开。
11. **HTTP 插件 PUT：** Resolver 能用到会话已有身份；插件结果不触发捕获。
12. **开箱：** 不设置 `BAIZE_CONNECTOR_TOKEN` 时，等价于空 static headers 的注册成功（单测或对 `ResolveDefaults` / Apply 的配置夹具）。

Chat UI：Tools 设置页开关的交互可用组件测试或手工验收；API 契约以上述后端测试为准。

---

## 11. 文档

- `README.md` / `README.zh-CN.md`：删掉 PUT 未挂 Identities / Resolver / Capture 的说明；写明对话用会话登录、配置 Token 可选且不用于带 `conversation_id` 的 Run；Tools 设置可标「需要登录」。
- `docs/architecture-and-plugin-protocol.md`：invoke 优先级与 §7 对齐；注明「需要登录」是操作员开关，不是 OpenAPI security 推断。

---

## 12. 实现时注意的类型与包

| 名称 | 作用 |
|------|------|
| `connector.Apply` / `ApplyInput` | start 与 PUT 的唯一注册入口 |
| `store.Connector.RequireLogin` | `[]string`，与 `RequireApproval` 并列 |
| `store.ConnectorAuth` 的 Capture | 配置形状；GET 回显 |
| `tool.Info.RequireLogin` | `require_login` |
| `PATCH /v0/tools/{name}` | 仅 `require_login` |
| `authBody.capture` | PUT JSON |

前端：`web/chat` 的 `ToolInfo` 与 Tools 设置页增加开关；不改聊天主区信息架构。
