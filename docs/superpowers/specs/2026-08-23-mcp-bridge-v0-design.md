# Baize 设计规格：MCP 桥 v0

> 状态：已批准  
> 日期：2026-08-23  
> 前置：HTTP 侧车插件 v0、工具目录、生产 `baize start` 干净部署已落地  
> 依据：头脑风暴（方案 B：stdio + HTTP 传输；官方示例仅 DB；搜索类 MCP 仅文档说明）

本规格把 **Model Context Protocol（MCP）** 接到 Baize Connector / Tool 目录 / Run 引擎，使管理员可注册 MCP Server（子进程 stdio 或远程 Streamable HTTP），工具进入统一目录，行为与 OpenAPI / HTTP 插件对齐（启停、HITL、`require_approval`、审计）。**不自研内置 DB 驱动**；数据库能力通过生态 MCP（官方 compose 示例推荐 DBHub）接入。搜索、浏览器、Slack 等任意 MCP 由使用者自行配置 URL 或 stdio 命令，本里程碑**不提供搜索 compose 示例**。

---

## 0. 动机

v1 生产路径已锁定为 **接 MCP 生态**（而非 Runtime 直连业务库）。设置页 `/settings/mcp` 仍为 `ComingSoon`；架构图已预留「MCP 桥」。OpenAPI 覆盖遗留 REST；HTTP 侧车覆盖定制业务；**MCP 覆盖 DB、搜索、SaaS 等社区 Server**，且不必为每种能力写 Baize 内置驱动。

---

## 1. 目标与成功标准

**目标：** 管理员通过 `PUT /v0/connectors/{id}`（`type: mcp`）或设置 → MCP 注册 Server；Runtime `tools/list` 发现工具并 `tools/call` 执行；工具在目录中 `source=mcp`，Tools 页可启停与改说明。

**成功标准：**

1. **stdio**：配置 `command` / `args` / `env` 可注册本地 MCP（集成测试用 mock MCP；compose 示例用 DBHub + demo Postgres）
2. **HTTP**：配置 `url`（+ 可选 `headers`）可注册 Streamable HTTP MCP（文档说明可接 Tavily 等远程 URL；**不提供搜索 compose**）
3. `GET /v0/tools` 含 MCP 工具行；`GET /v0/connectors/{id}` 回显 MCP 配置（密钥不落盘明文策略见 §5）
4. mock LLM Run 能选中并调用 MCP 工具；失败在 tool result 标 `is_error`，不直接 `UpdateRun(failed)`（与 http 插件一致）
5. `require_approval` 对 MCP 工具生效；`auth.mode` 对 MCP invoke **不适用**（MCP 鉴权在 Server 侧 env/headers）
6. 注册失败（进程起不来、list tools 失败、工具名全局冲突）→ `400 invalid_mcp` 或 `409 tool_conflict`，Registry 与目录保持失败前状态
7. 生产 `minimal.yaml` / `baize start` **不**预置 MCP Connector
8. 设置 → MCP 替代 `ComingSoon`：列表、添加/编辑、保存错误展示；admin ACL
9. README 中英：MCP 概念、stdio/HTTP 配置、DBHub compose 示例、**自行接入搜索 MCP 的说明**（无搜索 compose）
10. 集成测试：stdio mock MCP + 可选 DBHub compose 路径（CI 可只跑 mock）

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| Connector | `type: mcp`；传输 `stdio` \| `http`（Streamable HTTP） |
| 持久化 | `connectors` 表增 `mcp_json`（或等价列）存传输与连接配置 |
| 客户端 | 新包 `internal/connector/mcp`：`ListTools`、`CallTool`、stdio 子进程会话、HTTP 客户端 |
| Apply | 发现 → 合并目录 `source=mcp` → 仅注册 `enabled` 行 |
| Registry | invoke 走 MCP `tools/call`；stdio 长连接按 Connector 复用（见 §4.3） |
| API | `PUT` / `GET /v0/connectors/{id}` 扩展 body；错误码 `invalid_mcp` |
| GUI | `/settings/mcp`：表单 + 列表 + 链到 Tools |
| 示例 | `docker-compose.mcp-demo.yml`：Postgres + DBHub（stdio 或 HTTP 二选一，规格实现时定一种） |
| 文档 | README + 架构 §2 注明 MCP 桥已落地；**搜索 MCP 仅文字示例** |
| 测试 | mock MCP 单测/集成；注册失败不污染 |

### 不做

- 内置 SQL / DB 驱动
- MCP 市场、版本管理、一键安装 npm 包
- 搜索 MCP 的 compose / 开箱自动注册
- 每会话动态开关 MCP（Composer「+」不接 MCP）
- MCP 工具 `extra` 手加、DELETE 工具行（与 `plugin` 相同：仅启停）
- OpenAPI 登录捕获套在 MCP 上
- SSE 传输（若 Server 仅 SSE 无 Streamable HTTP，文档说明先用 stdio 或换 Server）
- `DELETE /v0/connectors/{id}`（与现网一致，本里程碑不新增）
- Run 事件 **Webhook**（另里程碑）

---

## 3. 架构

```
PUT /v0/connectors/{id}  type=mcp  transport=stdio|http
          │
          ▼
mcp.Register / Apply
  stdio: spawn MCP Server → initialize → tools/list
  http:  POST Streamable HTTP endpoint → tools/list
          │  ToolDesc[] → store.Tool (source=mcp)
          ▼
Run / ReAct
  tools/call(arguments, context)
  stdio: 同一 Connector 会话（可配置超时）
  http:   请求远端 MCP
```

与 OpenAPI / `http` 插件并列；未知 `type` 仍 `400 unsupported connector type`。

---

## 4. 注册与调用契约

### 4.1 PUT / GET body（`type: mcp`）

```yaml
connector:
  id: analytics-db
  type: mcp
  mcp:
    transport: stdio          # stdio | http
    # stdio 时：
    command: npx
    args: ["-y", "@bytebase/dbhub", "--transport", "stdio", "--dsn", "postgres://..."]
    env:
      KEY: "value"            # 可 env:VAR / file: 引用（与 auth 解析一致）
    # http 时：
    url: "https://example.com/mcp"   # Streamable HTTP 端点
    headers:
      Authorization: "Bearer ${SOME_TOKEN}"
  require_approval:
    - execute_sql
```

- `transport` 缺省 → `400 invalid_request`
- stdio 缺 `command` → `400`
- http 缺 `url` → `400`
- 不需要 `spec` / `base_url`（`base_url` 留空）
- `auth` 块对 MCP **忽略**（文档写明）；鉴权靠 `mcp.env` / `mcp.headers`

GET 回显时：`env` / `headers` 中已解析的密钥以占位或省略策略与现网 `auth` 一致（不新增泄露面）。

### 4.2 工具目录

- **`source: mcp`**（新增常量，与 `spec` / `plugin` / `extra` 并列）
- 发现字段：`name`、`description`、`input_schema`（来自 MCP tool `inputSchema`）
- `method` / `path` 空；Tools 树分组进 **「其他」**（与 HTTP 插件相同）
- 合并语义同 `plugin`：PUT 合并 `enabled`、人改 `title`/`description_custom`；消失的工具名删除行
- **不可** `POST /v0/connectors/{id}/tools` 手加；**不可** DELETE 行

### 4.3 stdio 进程生命周期（v0）

- **每个已注册 MCP Connector 在 Runtime 内保持一条 MCP 会话**（一个子进程 + JSON-RPC 循环），供 list 与 call 复用
- 再次 `PUT` 同一 `id`：关闭旧进程，起新进程，重新 list
- Runtime 退出：关闭所有 MCP 子进程
- 子进程崩溃：下次 invoke 尝试重启一次；仍失败 → tool `is_error`
- 超时：list/call 客户端超时默认 **30s**（与 http 插件一致），可配置项留 follow-up

### 4.4 HTTP 传输（Streamable HTTP）

- 实现 MCP 规范 **Streamable HTTP**（2025-03-26 及 Baize 依赖的 Go SDK 所支持版本）
- 用于：远程托管 MCP（文档示例：Tavily `https://mcp.tavily.com/mcp/?tavilyApiKey=...`）、内网 MCP 网关
- **不在仓库提供搜索 Server 部署**；README 给 1～2 个 URL/stdio 配置片段即可
- 可选 `headers` 用于 Bearer / 自定义鉴权

### 4.5 注册失败

| 场景 | 期望 |
|------|------|
| 命令不存在 / 进程立即退出 | `400 invalid_mcp` |
| `tools/list` 失败或 0 工具 | `400 invalid_mcp` |
| 工具名与已有 Connector 冲突 | `409 tool_conflict` |
| HTTP 非 MCP 端点 / 4xx | `400 invalid_mcp` |

### 4.6 Run 时 invoke

- 参数：MCP `arguments` 对象 ← Baize tool 调用 JSON
- 上下文：`run_id` / `agent_id` 可放入 MCP 元数据（若 SDK 支持 `_meta` 或等价；否则 v0 仅传 arguments）
- 返回：映射为 tool content + `is_error`（与 http 插件一致）
- HITL：仅 `require_approval` 名单；不读 MCP `annotations`

---

## 5. 安全与配置

- **stdio `env`**：支持 `env:VAR`、`file:/path` 引用（复用 `authcred` / `controlplane` 解析器），**禁止**在 GET 回显明文密钥
- **http `headers`**：同上；`${VAR}` 展开在注册时或调用时（规格实现时二选一，默认与 static auth 一致在注册时解析默认值）
- MCP Server 自身访问 DB / 外网的风险由 **Server 选型 + 只读账号 + 网络策略** 承担；Baize 文档推荐 DBHub 只读 DSN
- 控制面：PUT Connector 需 **admin**（与现网一致）

---

## 6. 设置页 MCP（GUI）

路径：`/settings/mcp`（admin only）。

- **列表**：已注册 `type=mcp` 的 Connector（从 `GET` 工具聚合 connector_id 或后续 `ListConnectors`；v0 可从工具目录去重 + `GET /v0/connectors/{id}` 拉详情）
- **添加/编辑**：`id`、`transport`、stdio 字段或 `url`+`headers`、`require_approval` 多行
- **保存**：`PUT /v0/connectors/{id}`；展示 `invalid_mcp` / `tool_conflict` 全文
- **测试连接**：保存即 list tools（与 PUT 同路径）；无单独 probe API（v0）
- 每行：**工具数**、链到 **设置 → Tools**
- 不实现：MCP 市场、npm 搜索、搜索 Server 一键模板

---

## 7. 官方示例（仅 DB）

### 7.1 文档推荐 DB MCP

- **DBHub**（[bytebase/dbhub](https://github.com/bytebase/dbhub)）：多引擎、stdio/HTTP、MCP Registry 收录；compose 示例默认用它
- 文档可简述：Google MCP Toolbox、Bytebase 平台 MCP 等备选，**不绑定实现**

### 7.2 `docker-compose.mcp-demo.yml`（试用，非 `baize start` 默认）

- 服务：`postgres`（demo 库 + 只读/读写账号说明）、`baize`（`minimal` 或 demo 配置 + 文档中的 PUT 片段）
- **不**包含搜索 MCP 容器
- 可选：DBHub 作为独立 HTTP 服务，Baize 用 `transport: http` 注册；或 Baize stdio 直接 `npx @bytebase/dbhub`（实现时选维护成本更低的一种并在 README 写清）

### 7.3 搜索 MCP（仅文档）

README 增加小节「接入搜索 / 联网 MCP」，**不含 compose**：

- 说明：Baize 不代理互联网；由 MCP Server 访问搜索 API
- **HTTP 示例**：Tavily 远程 MCP URL（用户自备 API Key）
- **stdio 示例**：Brave 官方 MCP `npx` 命令（用户自备 `BRAVE_API_KEY`）
- 强调：生产 `baize start` 后通过设置 → MCP 或 `PUT` 自行添加；工具名以 Server 为准，注意全局唯一

---

## 8. Store / 迁移

- SQLite `connectors` 表增加 `mcp_json TEXT`（JSON 存 `transport`、`command`、`args`、`env`、`url`、`headers`）
- 内存 Store 同步字段
- 旧库 `ALTER` 迁移；缺列启动时迁移（与 `tools.title` 同类模式）

`Connector` 结构体增加 `MCP` 字段（或内嵌 JSON 类型），`PUT`/`GET` JSON 字段名为 `mcp`。

---

## 9. 依赖

- 引入 **官方或社区 Go MCP SDK**（实现计划中选型，优先 Model Context Protocol 官方 Go 实现）
- 不引入 Node 运行时进 Baize 主二进制；DBHub 等仍由用户环境 `npx` 或 compose 侧车提供

---

## 10. 测试

| 类型 | 内容 |
|------|------|
| 单测 | mock MCP Server（stdio）：list + call；HTTP mock（httptest） |
| 集成 | PUT mcp → GET tools → POST run → tool.result |
| 回归 | mock-ticket / agent-skills 开箱路径不受影响 |
| 失败 | 坏 MCP 不污染 Registry；`invalid_mcp` 断言 |

---

## 11. 文档

- `docs/architecture-and-plugin-protocol.md` §2：MCP 桥标为已实现（本里程碑合并后）
- README 中英：新章节「MCP 连接器」+ DB compose + 搜索「自行接入」小节
- 不修改 `minimal.yaml` 默认

---

## 12. 规格自检

| 检查项 | 结果 |
|--------|------|
| 与「接生态、无内置 DB」一致 | ✅ |
| 传输范围 stdio + HTTP | ✅ |
| 无搜索 compose | ✅ |
| `source=mcp` 与 `plugin` 分离 | ✅ |
| 生产 start 不带 demo MCP | ✅ |
| Webhook 不在本里程碑 | ✅ |
| 删除 Connector API | 未承诺 |
| HITL / 目录合并 | 对齐 http 插件 |
| 安全：env/headers 不明文回显 | 已写 |

---

## 13. 实现顺序建议（供 writing-plans）

1. Store + `mcp_json` + `ToolSourceMCP`
2. `internal/connector/mcp`（stdio + HTTP）
3. `Apply` / `registerOne` 分流
4. API `PUT`/`GET` + `invalid_mcp`
5. 集成测试 mock MCP
6. `McpSettings.tsx` + `api.ts`
7. `docker-compose.mcp-demo.yml` + README
8. 架构文档更新

分支名建议：`feat/mcp-bridge`；实现方式：**子代理驱动**（与 Agent Skills 相同）。

---

*批准本规格后，调用 writing-plans 生成 `docs/superpowers/plans/2026-08-23-mcp-bridge-v0.md`，再进入实现。*
