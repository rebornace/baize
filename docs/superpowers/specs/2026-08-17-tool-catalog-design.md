# Baize 设计规格：Connector 工具目录

> 状态：已批准  
> 日期：2026-08-17  
> 前置：Chat UI 壳、按会话登录与「需要登录」、控制面操作员/管理员口令已落地  
> 依据：本轮头脑风暴（目录挂 Connector；方案 2 为 Store 中的 Tool 行；OpenAPI 可手加 REST；插件仅启停；GUI 在设置 → Tools）

本规格把「Agent 用哪些工具」收成 Connector 下的一份可编辑目录。不修改会话身份优先级、三种 Connector `auth.mode`、HITL、控制面口令。不给 Agent 再做一份白名单。

本规格**修订** `2026-08-15-session-login-tool-gate-design.md` §2 中「不把需要登录写入 SQLite」：Tool 行落盘后，`require_login` / `require_approval` / `enabled` 随行一起持久化。

---

## 0. 动机

导入 OpenAPI 或挂上 HTTP 插件之后，注册表里有多少 operation / 侧车工具，模型就能看到多少。管理员不能关掉用不上的，也不能补说明书漏写的一个 REST 接口。热更新和重启还会把设置页上改过的「需要登录」冲掉（Connector 至今只在内存里）。

配置白泽的人应当维护一份目录：多出来的不用，缺的可加。Agent 仍然使用**所有 Connector 上启用中的工具合集**（一个 Agent 不绑定 Connector 列表；多 Connector 时工具名全局唯一）。

---

## 1. 目标与成功标准

**目标：** Connector 拥有第一等公民的 Tool 目录（SQLite 落盘）。管理员可在 `/ui` 设置 → Tools 启停工具；OpenAPI Connector 可手加同一 `base_url` 上的 REST；HTTP 插件只能启停侧车发现的工具。换 Spec、开机合并都不得无故丢掉勾选和手加。

**成功标准：**

1. PUT OpenAPI 后目录行数等于 Spec 中的 operation；`source=spec`，默认启用
2. PATCH `enabled=false` 后模型 tool list 与 Registry 均无该名；再打开后恢复；`require_login` 仍在
3. POST extra 后按 method+path 打到该 Connector 的 `base_url`；带 `conversation_id` 时走会话身份，不走 Connector 默认头
4. DELETE `spec`/`plugin` 行 → 400；DELETE extra → 行与 Registry 均消失
5. 省略目录字段的 PUT：同名停用仍停用；extra 仍在；新 operation 默认启用；Spec 中消失的 spec 行删除
6. SQLite：停用 + extra → 重启进程后仍在；YAML 未写目录字段
7. PUT `type: http` 后可停用侧车工具；对该 Connector POST extra → 400
8. 手加名冲突 → 409；Apply 失败时目录与 Registry 不部分更新
9. 操作员 GET/PATCH/POST/DELETE 目录 → 403；管理员可以
10. 现有 mock-ticket 集成测试仍通过（开箱全启用，与今天四把工具一致）

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| Store | `Tool` 行（见 §4）；SQLite 落盘 **Connector + Tool**（今天两者只在内存） |
| 注册 | `connector.Apply` 先发现、再与已有行合并、再按启用行重建 Registry |
| API | `GET /v0/tools` 返回目录（含停用）；`PATCH` 增加 `enabled`；`POST/DELETE /v0/connectors/{id}/tools` |
| GUI | 设置 → Tools：启用开关、手加表单、删除 extra；失败页内报错 |
| 门禁 | 新接口最低角色管理员；未知 `/v0` 仍默认管理员 |
| 文档 | README 中英；架构草案：目录行与 Registry 派生关系 |

### 不做

- Agent 绑定 Tool / Connector 子集
- MCP 真连接、插件登录捕获、插件 Connector 上手加 REST
- PATCH 改 `require_approval`（设置页审批徽章仍只读；名单仍由 YAML / PUT 写入行）
- 工作流 DSL、Webhook、SDK、Channel、企业回调
- 多实例 / 多机高可用（仍是 SQLite 单节点）
- Connector 可视化编辑器（改 Spec / `base_url` 仍走 YAML 或 PUT）

---

## 3. 产品行为

### 3.1 谁能改

| 角色 | 目录 |
|------|------|
| 管理员 | GUI 与 API 全开 |
| 操作员 | 不能进 Tools 设置；目录接口 403 |
| 门关着 | 与今天一样：能打到控制面就能改（无口令时无角色限制） |

### 3.2 启停 vs 删除

| `source` | 启停 | 删除 |
|----------|------|------|
| `spec`（OpenAPI operation） | 可以 | 不可以（400） |
| `plugin`（侧车发现） | 可以 | 不可以（400） |
| `extra`（手加 REST） | 可以 | 可以 |

停用：不进入 Registry，模型选不到；按名 invoke 视为未知工具，不打下游。设置页仍列出停用行，以便再打开。

### 3.3 手加（仅 `type: openapi`）

同一 Connector、同一 `base_url`。管理员提供：name、HTTP 方法、路径、说明、`input_schema`（JSON 对象，可为 `{}`）。调用与 OpenAPI 转出的工具相同：默认头 / 会话身份 / `require_login` / HITL。

HTTP 插件 Connector 上手加 → 400。缺的 REST 应加在对应的 OpenAPI Connector 上，或以后在侧车里暴露工具。

### 3.4 多 Connector

进程可有多个 Connector；Agent **没有** Connector 名单。每次 Run 把 Registry 里全部启用工具交给模型。跨 Connector 工具名仍全局唯一，冲突拒绝。

### 3.5 开箱

`configs/default.yaml` 不预置停用或手加。mock-ticket 四把工具全部启用，30 秒跑通不变。

---

## 4. 数据模型

### 4.1 `store.Tool`

| 字段 | 含义 |
|------|------|
| `connector_id` | 所属 Connector |
| `name` | 工具名；全进程唯一 |
| `source` | `spec` \| `extra` \| `plugin` |
| `enabled` | 默认 `true` |
| `description` | 给模型看的说明 |
| `method` / `path` | OpenAPI / extra 必填；plugin 可空 |
| `input_schema` | JSON Schema 对象 |
| `require_login` / `require_approval` | 挂在行上，不再以 Connector 字符串数组为真相 |
| `operation_id` | spec 行可填；extra / plugin 可空 |

Memory 与 SQLite 同一接口。SQLite 新增 `connectors` 与 `tools` 表（或等价列）。现有库无这两张表时按空目录处理，再走合并。

Connector 仍保存：id、type、spec、base_url、auth。`require_login` / `require_approval` 数组仅作 YAML/PUT **输入**，Apply 后写进行；`GET /v0/connectors/{id}` 可继续回显由行聚合的名单，避免调用方断裂。

### 4.2 Registry

只注册 `enabled=true` 的行。引擎、HITL、登录门闸继续读 Registry，不直接扫目录。`GET /v0/tools` **改为读目录**，因此能看见停用行。

---

## 5. 合并规则（PUT 与开机相同）

发现来源：OpenAPI → Spec 的 operation；插件 → 侧车 `GET /v0/tools`。

对该 Connector 已有行：

1. 发现列表里**新出现**的 `spec`/`plugin` 名：插入，`enabled=true`，门闸按本次 YAML/PUT 名单（省略登录名单则 `require_login=false`）
2. 发现列表里**仍在**的同名：保留 `enabled` 与行上门闸；若本次 PUT/YAML **显式**带了 `require_login` / `require_approval` 数组，则按名单重写这两位（空数组 = 该 Connector 下全 false）
3. 发现列表里**消失**的 `spec`/`plugin` 行：删除
4. `source=extra`：除非 DELETE，否则保留（含启用与门闸）
5. YAML / PUT **省略**目录相关字段（不停用名单、不手加列表、省略 `require_login`）：步骤 2 的门闸保留行上已有值

开机：YAML 仍提供默认 Connector 的 spec/auth。若 SQLite 已有该 id 的 Connector 与 Tool 行，按上表合并，**不得**因 YAML 未写目录而把全部改回启用。YAML 里登记过、库里也有的**其他** Connector id：从库恢复（否则热更新过的第二套系统重启即丢）。YAML 未出现、仅存在于库中的 Connector：开机也要加载（与「目录进 Store」一致）。

Apply 失败（坏 Spec、`invalid_auth`、插件不可达、撞名）：Store 中该次拟写入回滚；Registry 保持失败前。不得出现「Connector 已更新但工具只注册了一半」。

---

## 6. 控制面 API

新路由一律加入控制面 ACL，最低角色 **管理员**。

### 6.1 `GET /v0/tools`

返回 `{ "tools": [ ... ] }`。元素为目录行，含 `enabled`、`source`，以及现有 name、connector_id、method、path、description、input_schema、require_login、require_approval。按 name 排序（与今天 List 一致）。

### 6.2 `PATCH /v0/tools/{name}`

```json
{ "enabled": false, "require_login": true }
```

`enabled` 与 `require_login` 均为可选指针；**至少出现一个**。只改给定字段。未知名 → 404 `not_found`。非法 JSON / 两个字段都缺 → 400 `invalid_request`。成功：更新行，若 `enabled` 变化则增删 Registry 注册；响应为更新后的目录行（形状与 GET 单条相同）。

不在本接口改 `require_approval`、name、path。

### 6.3 `POST /v0/connectors/{id}/tools`

```json
{
  "name": "get_device",
  "description": "按 id 查设备",
  "method": "GET",
  "path": "/devices/{id}",
  "input_schema": { "type": "object", "properties": { "id": { "type": "string" } } },
  "require_login": false,
  "require_approval": false
}
```

仅 `type=openapi`。`name` 非空；method ∈ GET/POST/PUT/PATCH/DELETE；path 必须以 `/` 开头；`input_schema` 必须是 JSON 对象。`require_login` / `require_approval` 缺省 false。插件或未知 type → 400 `invalid_request`。重名 → 409 `conflict`。未知 Connector → 404。成功 200，体为新行；Registry 立即注册（默认 enabled）。

### 6.4 `DELETE /v0/connectors/{id}/tools/{name}`

仅 `source=extra`。否则 400 `invalid_request`。成功 204，行删除并从 Registry 卸下。

### 6.5 `PUT /v0/connectors/{id}` / `GET /v0/connectors/{id}`

PUT 不传工具目录则按 §5 合并。GET 的 `tools` 改为该 Connector 的目录行（含停用），不是只含 Registry。

`POST /v0/runs` 行为不变：模型只看到启用工具。

---

## 7. GUI

设置 → Tools（已有管理员导航）：

- 每行：现有「需要登录」+「需审批」徽章 + **启用**开关
- `source=extra` 显示删除；`spec`/`plugin` 无删除
- 「添加工具」表单：name、method、path、description、input_schema（JSON 文本，空则 `{}`）；提交 POST
- 加载用 GET `/v0/tools`；错误文案留在页内
- 不改聊天主区、MCP/插件空页、账号页

手加 Schema 不做可视化 Schema 编辑器。非法 JSON 在提交前或由 API 400 提示。

---

## 8. 调用

extra 与 spec 行共用 OpenAPI 执行路径：`base_url + path`、Connector 鉴权、会话身份、`require_login`、HITL。path 中的 `{param}` 从 arguments 取值的规则与现有 OpenAPI 工具一致（有则替换；本里程碑不新发明路由 DSL）。

plugin 行仍走侧车 invoke。停用行不注册，无调用闭包。

---

## 9. 错误

| 情况 | HTTP | code |
|------|------|------|
| 非法 JSON、缺 PATCH 字段、非法 method/path/schema、对插件手加、删除非 extra | 400 | `invalid_request` |
| 鉴权解析失败 | 400 | `invalid_auth` |
| Connector 或工具不存在 | 404 | `not_found` |
| 工具名冲突 | 409 | `conflict` |
| 操作员打目录接口 | 403 | `forbidden` |
| 无控制面口令（门开着） | 401 | `unauthorized` |

Invoke 未知或已停用名：与今天 Registry 未注册相同，不新增错误码。

---

## 10. 测试

至少覆盖 §1 的 10 条。Memory + SQLite 都要测合并与重启（SQLite 测落盘；Memory 测进程内 PATCH/POST 后 GET）。

控制面 ACL 表驱动补上 `POST/DELETE .../tools`。现有 `tests/integration` 开箱路径不得因默认停用而失败。

UI：组件测试覆盖启用开关与 extra 删除按钮出现条件即可；完整表单可手工。

---

## 11. 文档

- `README.md` / `README.zh-CN.md`：设置 → Tools 可启用/停用；OpenAPI 可补 REST；插件只能停用；SQLite 下重启保留。与下游登录、控制面口令分开写。
- `docs/architecture-and-plugin-protocol.md`：Tool 为 Connector 目录行；Registry 仅启用行；Agent 用合集。
- `configs/default.yaml` / `docker.yaml`：不增加必填目录字段。

---

## 12. 实现时注意的类型与包

| 名称 | 作用 |
|------|------|
| `store.Tool` | 目录行 |
| `store.ListTools` / `GetTool` / `UpsertTool` / `DeleteTool` | 目录 CRUD；按 connector 列出 |
| SQLite `connectors` / `tools` | 落盘；开机加载非 YAML 的 Connector |
| `connector.Apply` | 发现 → 合并 → 落盘 → 重建 Registry |
| extra 的 Invoker | 复用 OpenAPI HTTP 执行，方法/路径来自行 |
| `controlplane` ACL | POST/DELETE `/v0/connectors/{id}/tools` |
| `web/chat` ToolsSettings | 启用、添加、删除 extra |

Git：不在 `main` 上改实现代码。规格与计划批准后再开 `feat/tool-catalog`。

---

*本文档经头脑风暴分节批准后落盘；实现前若字段名有微调，以本文语义为准并更新本文，不静默漂移。*
