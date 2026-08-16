# Baize 设计规格：控制面操作员 / 管理员口令门禁

> 状态：已批准  
> 日期：2026-08-16  
> 前置：Chat UI 壳、按会话登录与工具「需要登录」已落地  
> 依据：本轮头脑风暴（薄控制面两扇门；两把可选静态口令；方案 1 中间件 + 路由最低角色表）

本规格不修改会话身份、`require_login`、Connector `static` / `passthrough` / `vault_ref`、HITL。那是打**下游业务 API** 的凭证；本里程碑是打**白泽自己的 HTTP 控制面**的门。

---

## 0. 动机

设置页已经能改「需要登录」，`PUT /v0/connectors` 能热替换整条接入。谁能打到 `:8080`，谁就能聊天，也能改白泽。配置白泽和用白泽聊天应当是两扇门。

本里程碑只做薄门禁：两把可选静态口令，管理员是操作员的超集。不做 SSO、用户表、按人隔离对话。同一角色的口令是共享的：所有操作员共用一把钥匙，能看见进程内全部对话。这是有意的 YAGNI。

---

## 1. 目标与成功标准

**目标：** 配了口令之后，操作员只能对话 / 审批 / 管本会话账号；管理员才能改 Agent、Connector、Tools。没配口令时，行为与今天完全相同。

**成功标准：**

1. 两把口令都空：现有测试与 `baize start` → `/ui` 不出现解锁页；API 不要求 `Authorization`
2. 配了口令：无口令或口令错 → `/v0` 业务接口 `401`；操作员口令打配置接口 → `403`；管理员口令可以聊天也可以改配置
3. `/ui` 先解锁再进聊天；操作员看不到 Tools / MCP / 插件；账号页仍可用
4. 口令不出现在 Run 事件、SSE、GET run、日志
5. 门开着时，不带 `conversation_id` 的机器 / curl 同样要带操作员或管理员口令才能打控制面

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 配置 | `control_plane.operator_token` / `admin_token`；`default.yaml` 与 `docker.yaml` 默认空 |
| 中间件 | 一层鉴权；路由表声明最低角色；未列入的 `/v0` 默认管理员 |
| 角色 | `admin` 是 `operator` 的超集 |
| 接口 | `GET /v0/ui-config` 增加 `gate_enabled`；新增 `GET /v0/me` |
| UI | 解锁页；`localStorage` 键 `baize.control_token`；请求带 Bearer；SSE 改 `fetch`；按角色显隐设置导航 |
| 文档 | README 中英；架构草案控制面补一句 |

### 不做

- SSO、OAuth、用户表、注册、Cookie 会话、多租户 IAM
- 按操作员隔离对话 / Run（共享口令 = 共享可见性）
- 把口令放进 query 或 `WWW-Authenticate` Basic 弹窗
- 改会话登录、`require_login`、HITL、Connector 三种 mode
- MCP 真连接、工具增删目录、HTTP 插件登录捕获
- 工作流 DSL、Webhook、SDK

---

## 3. 配置

```yaml
control_plane:
  operator_token: ""   # 或 env:BAIZE_OPERATOR_TOKEN 或 file:/path
  admin_token: ""      # 或 env:BAIZE_ADMIN_TOKEN
```

解析：

- 空字符串或未配 = 这把口令不存在
- `env:NAME` / `file:path`：格式与 Connector `vault_ref` 相同
- **与 vault_ref 的差别：** `env:` 对应环境变量为空或未设置时，视为「这把口令未配」，**不**导致进程启动失败，也**不**返回 `invalid_auth`
- 允许 YAML 明文（供测试）；生产文档只推荐 `env:` / `file:`，不要把密钥写进 YAML

**门是否打开：** 解析后至少有一把非空口令 → `gate_enabled=true`。一把都没有 → 中间件对所有请求放行。

**只配一把：**

| 配了什么 | 效果 |
|----------|------|
| 只有管理员 | 这把口令角色为 `admin`；没带口令 `401`；没有「仅操作员」身份 |
| 只有操作员 | 这把口令角色为 `operator`；打管理员接口 `403`；配上管理员口令之前没人能改配置 |

两把解析后的值相同且非空：对上即视为 `admin`。文档写明：要两扇门就用两个不同值。

启动时把解析后的口令注入 API Server；不把明文写进 Store、GET Connector、`ui-config`。

---

## 4. 鉴权中间件

`Server.Handler()` 包一层门禁，再交给现有 mux。不拆端口、不改现有 URL。

**公开（不验口令，门开着也放行）：**

- `GET /healthz`
- `GET /v0/ui-config`
- `/ui/` 静态资源（含 SPA fallback）

其余 `/v0/*`：门关着则放行；门开着则：

1. 读取 `Authorization: Bearer <token>`。缺少前缀、空 token、其它头名 → 按无口令处理
2. 不接受 query、Cookie、Basic
3. 对已配置的口令做恒定时间比较：先管理员（若有），再操作员（若有）
4. 对上管理员 → 角色 `admin`；否则对上操作员 → `operator`；否则 `401`
5. 将角色放入 request context，供 `GET /v0/me` 使用
6. 查路由最低角色：够则放行；有效 `operator` 打管理员路由 → `403`

未知的 `/v0` 方法 + 路径：**默认最低角色为管理员**（对操作员失败关闭）。

---

## 5. 路由 ACL

| 最低角色 | 接口 |
|----------|------|
| 公开 | `GET /healthz` · `GET /v0/ui-config` · `/ui/` |
| 操作员（管理员也能调） | `GET /v0/me` · `POST /v0/runs` · `POST /v0/runs/{id}/resume` · `GET /v0/runs/{id}` · `GET /v0/runs/{id}/events` · `GET /v0/runs/{id}/stream` · `GET /v0/conversations` · `GET /v0/conversations/{id}/messages` · `DELETE /v0/conversations/{id}/messages` · `GET /v0/conversations/{id}/identities` · `POST /v0/conversations/{id}/identities/{iid}/default` · `DELETE /v0/conversations/{id}/identities/{iid}` · `DELETE /v0/conversations/{id}/identities` |
| 仅管理员 | `PUT /v0/agents/{id}` · `PUT /v0/connectors/{id}` · `GET /v0/connectors/{id}` · `GET /v0/tools` · `PATCH /v0/tools/{name}` |

本表以外的新 `/v0` 接口在实现时必须加入表；漏加则操作员会被拒、管理员仍可过。

---

## 6. 给 UI 的接口与错误体

**`GET /v0/ui-config`（公开）：**

```json
{ "agent_id": "ticket-agent", "gate_enabled": false }
```

`gate_enabled` 为 JSON 布尔，不是字符串。不回显是否配置了哪一把口令。现有只含 `agent_id` 的客户端多一个字段仍可解析。

**`GET /v0/me`：**

门开着：最低操作员。

```json
{ "role": "operator" }
```

`role` 仅为 `"operator"` 或 `"admin"`。

门关着：不要求 `Authorization`，返回 `{ "role": "" }`。

**错误（沿用 `{ "error": { "code", "message" } }`）：**

| 情况 | HTTP | code | message |
|------|------|------|---------|
| 门开着，没带或口令不对 | 401 | `unauthorized` | `需要控制面口令` |
| 操作员打管理员接口 | 403 | `forbidden` | `需要管理员口令` |

401 不附带 `WWW-Authenticate`。错误口令与缺口令同一 `code`，不区分「哪一把错了」。

SSE：握手阶段就验完。失败时普通 JSON 错误，`Content-Type` 不得是 `text/event-stream`。

---

## 7. `/ui` 行为

**启动：**

1. 请求公开的 `GET /v0/ui-config`
2. `gate_enabled=false`：进入聊天；后续 `/v0` **不带** `Authorization`（忽略 localStorage 残留口令）
3. `gate_enabled=true` 且没有口令：全屏解锁页，进不了聊天或设置

**解锁页：** 一个口令框 + 确认，没有用户名。口令写入 `localStorage` 键 `baize.control_token`（与 `baize.conversation_id` 分开）。然后 `GET /v0/me`：401 则删除刚存的口令并显示「口令不对」；成功则按 `role` 进入聊天。

**已解锁：** 所有 `/v0` 的 `fetch` 带 `Authorization: Bearer …`。

**SSE：** 浏览器 `EventSource` 不能自定义头。`openRunStream` 改为 `fetch` 读取 `text/event-stream`（`after=` 事件下标仍用 query，不是秘密）。门关着也走同一条 `fetch` 路径，不保留两套客户端。解析 `id:` / `event:` / `data:` 的语义与现有 EventSource 客户端一致（含 `run.ended`）。

**按角色显隐：**

| | 操作员 | 管理员 |
|--|--------|--------|
| 左下角 | 「账号」→ `/settings/identities` | 「设置」→ 现有设置（默认 Tools） |
| 设置导航 | 只有「账号」 | Tools / 账号 / MCP / 插件（后两项仍为空状态） |
| 直开 `/settings/tools`、`/settings/mcp`、`/settings/plugins` | 前端重定向到 `/settings/identities`；API 仍 403 | 照常 |

聊天页提供 **退出控制面**：删除 `baize.control_token`，回到解锁页。这不是退出下游业务会话账号。

本里程碑接受口令在 localStorage（本机侧车）。不做 Cookie。

---

## 8. 与其它凭证的关系

| 凭证 | 作用 |
|------|------|
| 控制面口令（本规格） | 谁可以打白泽 `/v0` 和打开 `/ui` 业务面 |
| 会话 Identity / `require_login` | 对话里调下游 API 用谁的业务账号 |
| Connector `static` / `passthrough` / `vault_ref` | 无 `conversation_id` 的机器路径默认下游头 |

三者独立。控制面角色**不**自动变成下游 Identity，也**不**替代 `require_login`。

---

## 9. 测试

门关着时现有 `go test ./...` 必须仍通过。

至少覆盖：

1. 两把口令都空：不带 `Authorization` 的 `POST /v0/runs`、`PUT /v0/connectors/{id}` 成功
2. 门开着、无口令：`POST /v0/runs` → 401 `unauthorized`；`GET /healthz` 与 `GET /v0/ui-config` 仍 200，且 `gate_enabled=true`
3. 操作员口令：`POST /v0/runs` 成功；`GET /v0/me` 为 `operator`；`GET /v0/tools` 与 `PUT /v0/connectors/{id}` → 403 `forbidden`
4. 管理员口令：`GET /v0/me` 为 `admin`；`GET /v0/tools` 与 `PUT` Connector 成功
5. 错误口令 → 401，code 与缺口令相同
6. 只有操作员口令：该口令 `PUT` → 403
7. 两把相同：对上即为 `admin`（`GET /v0/tools` 成功）
8. SSE：带操作员或管理员口令可挂上 stream；无口令握手失败且 `Content-Type` 不是 `text/event-stream`
9. 事件 JSON 不含控制面口令明文
10. `env:BAIZE_OPERATOR_TOKEN` 在环境变量未设置时进程仍能启动，且 `gate_enabled` 仅由实际解析出的非空口令决定
11. 门关着：`GET /v0/me` 不要求头，`role` 为 `""`

ACL 用「角色 × 接口」表驱动测试，避免漏标。UI 解锁与导航可用组件测试或手工验收；API 契约以后端测试为准。

---

## 10. 文档

- `README.md` / `README.zh-CN.md`：控制面口令可选；配了之后 `/ui` 先解锁；操作员只能对话和账号，改 Tools 要管理员口令；门开着时 curl 示例带 `Authorization: Bearer …`。写清这与下游业务登录不是一回事。
- `docs/architecture-and-plugin-protocol.md` §3：可选控制面口令（操作员 / 管理员），不是下游业务 IAM。
- `configs/default.yaml` 与 `configs/docker.yaml`：注释中的 `control_plane` 两行，值保持空。

---

## 11. 实现时注意的类型与包

| 名称 | 作用 |
|------|------|
| `config.Config.ControlPlane` | `OperatorToken` / `AdminToken` 字符串（可为 `env:` / `file:` / 明文 / 空） |
| 启动解析 | 空 `env:` **不得**当作 Connector 那种启动失败；得到两把可能为空的明文，交给 Server |
| `api.Gate`（或等价中间件） | 比较口令、写 context 角色、查 ACL |
| `Server.Handler` | 返回 `Gate(mux)`，而不是裸 mux |
| `GET /v0/me` | `{ "role": "operator" \| "admin" \| "" }` |
| `ui-config` | 结构体：`agent_id string`、`gate_enabled bool` |
| `web/chat` | 解锁页、`api.ts` 统一带头、`openRunStream` 改 fetch、设置导航按角色 |

前端：`web/chat`；构建仍输出 `internal/ui/dist`。

Git：不在 `main` 上改代码。规格与计划批准后再开 `feat/control-plane-gate`。

---

*本文档经头脑风暴分节批准后落盘；实现前若字段名有微调，以本文语义为准并更新本文，不静默漂移。*
