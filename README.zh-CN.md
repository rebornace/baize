# Baize（白泽）

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

[English](README.md) | **中文**

**不改你的服务，旁挂一层就能用上 AI。卸掉不留痕。**

白泽是旁挂在现有 HTTP API 旁边的 Agent Runtime：业务进程不用改、不用嵌 SDK。有 OpenAPI 就自动变成工具；没有就加一个小 HTTP 插件。写操作可人工审批，用完把进程停掉即可。

> 侧车，不是聊天机器人：主路径是 **OpenAPI → Tools → HTTP**。`/ui` 是操作员控制台（对话列表、工具卡片、HITL），不是面向终端用户的聊天产品。会话级凭证按需启用。

---

## 为什么需要白泽

| 痛点 | 白泽做法 |
|------|----------|
| 服务端已经在跑，没有 AI，也不想动代码 | 旁挂 Runtime，Connector 把接口变成 Tool |
| 没有 Swagger / OpenAPI | HTTP 插件侧车 |
| LLM 直接打写接口不放心 | `require_approval` |
| 接完拆不干净、绑死平台 | 单进程、配置在外，停掉即走 |

### 五个概念

| 概念 | 含义 |
|------|------|
| **Runtime** | 承载 Agent、Tool、存储与控制面 HTTP 的进程 |
| **Agent** | 提示词 + LLM，驱动一次 Run |
| **Tool** | 可调用能力（通常对应一个 OpenAPI operation） |
| **Connector** | Spec + `base_url`（+ 鉴权）到 Tool 注册表的绑定 |
| **Run** | 一次执行：输入 → ReAct → 输出 / 待审批 / 失败 |

---

## 30 秒跑通（试用）

**环境要求：** Go 1.22+（无需 C 编译器；SQLite 为纯 Go）

Windows（仓库根目录，无需把 `baize` 装进 PATH）：

```powershell
.\demo.cmd          # 试用：mock LLM + 演示 HTTP，无需 Key
.\start.cmd         # 生产：须先配置 BAIZE_API_KEY（见下）
.\baize.cmd demo    # 与 demo.cmd 等价
.\baize.cmd start   # 与 start.cmd 等价
```

POSIX：

```bash
git clone https://github.com/rebornace/baize.git
cd baize
./scripts/demo.sh   # 或 go run ./cmd/baize demo
```

试用（`demo`）：

`baize demo` / `demo.cmd` 使用 `configs/demo.yaml`：mock LLM + 内嵌演示 HTTP，**无需 API Key**。不是生产默认路径。

> 说明：在未 `go install` 或加入 PATH 前，不能直接敲 `baize start`；请用 `go run ./cmd/baize start`、`.\start.cmd` 或 `.\baize.cmd start`。

- Runtime：`http://127.0.0.1:8080`（`/ui`）
- 演示 HTTP：`http://127.0.0.1:18080`
- LLM：内置 `mock`

若端口被占用，先结束旧的 `baize` 进程再启动。

### 操作员界面（`/ui`）

打开 `http://127.0.0.1:8080/ui`。若配置了控制面口令，打开 `/ui` 先解锁；操作员只能进账号，改 Tools 需要管理员口令。

- 左侧：对话列表 + **新对话**；左下角 **设置**（操作员显示「账号」）
- 主区：消息流；写工具以**卡片**展示（名称 + 状态），展开可见参数 / 结果
- `waiting_human`：在卡片上 **批准 / 驳回**（没有底部大横幅）
- 设置 → Tools（仅管理员）：按 Connector / 路径前缀折叠，可搜索；可改显示名和说明（换 spec 保留人改）；添加在抽屉；`extra` 可删；账号页操作员可用；设置 → MCP（仅管理员）可注册 MCP Server；HTTP 插件 Connector 仍通过 `PUT` 手动注册
- 设置 → Skills（仅管理员）：列出已安装包、上传 `.md` / `.zip`、删除用户包，并勾选默认 Agent 的 skills
- 进行中的 Run 走 SSE（`GET /v0/runs/{id}/stream`）；断流后 UI 回退为 700ms 轮询

内置 mock LLM 下，可发送「VPN 挂了，请建一条记录」，并在卡片上批准 `create_ticket`。

### 调用演示写接口（curl）

`POST /v0/runs` 为异步，立即返回 `run_id`。演示里工具名 `create_ticket` 默认要审批，状态为 `waiting_human`。

```bash
curl -s -X POST http://127.0.0.1:8080/v0/runs \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"ticket-agent\",\"input\":\"VPN 挂了，请建一条记录\",\"conversation_id\":\"conv_example_1\"}"
```

```bash
curl -s http://127.0.0.1:8080/v0/runs/<run_id>
```

### HITL 审批

这个演示写接口被标了审批。

```bash
# 批准
curl -s -X POST http://127.0.0.1:8080/v0/runs/<run_id>/resume \
  -H "Content-Type: application/json" \
  -d "{\"decision\":\"approve\",\"comment\":\"ok\"}"

# 驳回
curl -s -X POST http://127.0.0.1:8080/v0/runs/<run_id>/resume \
  -H "Content-Type: application/json" \
  -d "{\"decision\":\"reject\",\"comment\":\"nope\"}"
```

默认 `configs/demo.yaml` 使用 SQLite；Runtime 重启后，仍可对 `waiting_human` 的 Run 继续 `resume`。

```bash
curl -s http://127.0.0.1:18080/tickets
curl -s http://127.0.0.1:8080/v0/runs/<run_id>/events
# 实时轨迹（SSE），Ctrl+C 结束
curl -N http://127.0.0.1:8080/v0/runs/<run_id>/stream
```

若 `control_plane` 配置了口令，上述 `/v0` 请求需带：

`Authorization: Bearer <操作员或管理员口令>`

改 Connector / Tools 只能用管理员口令。这与对话里登录下游系统不是同一把钥匙。

---

## 接到你的服务

### 有 OpenAPI

把你的 HTTP 服务的 OpenAPI 注册成 Tools。

1. 准备 OpenAPI 文件与 Runtime 可访问的 `base_url`。
2. 注册 / 替换 Connector：

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/ticket-api \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"openapi\",\"spec\":\"examples/mock-ticket/openapi.yaml\",\"base_url\":\"http://127.0.0.1:18080\",\"require_approval\":[\"create_ticket\"]}"
```

3. 核对 Tools：

```bash
curl -s http://127.0.0.1:8080/v0/tools
```

应看到 `method` / `path` / `operation_id` / `connector_id`。

4. 通过 `POST /v0/runs` 或打开 `/ui` 跑一次。

同 `id` 再 `PUT` 会与该 Connector 已有目录**合并**：原 `spec`/`plugin` 行保留 `enabled` 与 `require_login`，消失的 operation 删除，新 operation 默认启用，`extra` 行除非显式 DELETE 否则保留。省略目录相关字段（`require_login` / `require_approval` / `tools`）则保持磁盘目录不变。坏 Spec 返回 `400`，不污染已有 Registry 或目录。

### 无 OpenAPI：HTTP 插件

当你的 HTTP 服务没有可用的 OpenAPI 规格时，可运行实现
`GET /healthz`、`GET /v0/tools`、`POST /v0/tools/{name}/invoke`
（`X-Baize-Protocol: v0`）的侧车。

```bash
go run ./examples/http-plugin/cmd/http-plugin
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/legacy-sidecar \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"http\",\"base_url\":\"http://127.0.0.1:19090\",\"require_approval\":[\"create_ticket\"]}"
```

HITL 仍使用 `require_approval`。试用 OpenAPI Connector 请用 `baize demo`。

### MCP Connector（可选）

[MCP](https://modelcontextprotocol.io/) Server 通过 **stdio**（本地子进程）或 **Streamable HTTP**（远程 URL）暴露工具。用 `PUT /v0/connectors/{id}`（`type: mcp`）或 **设置 → MCP**（管理员）注册；发现的工具写入目录 `source: mcp`，启停、HITL `require_approval`、Run 调用与 OpenAPI / HTTP 插件一致。Connector 上的 `auth` 块**忽略**，鉴权写在 `mcp.env`（stdio）或 `mcp.headers`（HTTP）。生产 `baize start` **不**预置任何 MCP Server。

**stdio — 本地子进程**（`npx` 命令需在 **与 Baize 同一台宿主机** 上执行）：

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/analytics-db \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"mcp\",\"mcp\":{\"transport\":\"stdio\",\"command\":\"npx\",\"args\":[\"-y\",\"@bytebase/dbhub\",\"--transport\",\"stdio\",\"--dsn\",\"postgres://baize:baize@127.0.0.1:5432/demo?sslmode=disable\"]},\"require_approval\":[\"execute_sql\"]}"
```

**HTTP — Streamable HTTP 端点**（远程 MCP，或 Baize 在容器内、Server 在宿主机时）：

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/tavily \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"mcp\",\"mcp\":{\"transport\":\"http\",\"url\":\"https://mcp.tavily.com/mcp/?tavilyApiKey=YOUR_KEY\"}}"
```

配置错误或 Server 不可达 → `400 invalid_mcp`（Registry 不变）。工具名全局冲突 → `409 tool_conflict`。

#### MCP + Postgres 试用（`docker-compose.mcp-demo.yml`）

仅 Postgres 演示库 + Baize Runtime — **不含** Node/DBHub 容器，**不**预注册 MCP：

```bash
export BAIZE_API_KEY=sk-...
docker compose -f docker-compose.mcp-demo.yml up --build
```

- Postgres：`postgres://baize:baize@127.0.0.1:5432/demo`（`examples/mcp-demo/init.sql` 含示例 `tickets` 表）
- Runtime / UI：http://127.0.0.1:8080 （`/ui`）

Baize 镜像不含 Node，请在**宿主机**（Node 18+）运行 [DBHub](https://github.com/bytebase/dbhub)，再 `PUT` 注册：

```bash
# 宿主机 — stdio DBHub 连 compose Postgres（Baize 也需跑在宿主机）
npx -y @bytebase/dbhub --transport stdio --dsn "postgres://baize:baize@127.0.0.1:5432/demo?sslmode=disable"
```

用上方 stdio `PUT` 片段（`command` / `args`）注册。Baize 在 **Docker 内**时，在宿主机以 HTTP 模式起 DBHub，再注册 `transport: http`，例如 `url: http://host.docker.internal:9090/mcp`（端口/路径以 DBHub 文档为准）。

生产环境建议只读 DSN；compose 凭据仅供本地试用。

#### 搜索 / 联网 MCP（仅文档）

白泽不代理公网 — 由 MCP Server 访问搜索 API。**无**搜索 compose；`baize start` 后自行在设置 → MCP 或 `PUT` 添加。

**Tavily（HTTP）** — 远程 Streamable HTTP，API Key 写在 URL：

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/tavily \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"mcp\",\"mcp\":{\"transport\":\"http\",\"url\":\"https://mcp.tavily.com/mcp/?tavilyApiKey=YOUR_KEY\"}}"
```

**Brave Search（stdio）** — 宿主机 `npx`，自备 `BRAVE_API_KEY`：

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/brave-search \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"mcp\",\"mcp\":{\"transport\":\"stdio\",\"command\":\"npx\",\"args\":[\"-y\",\"@modelcontextprotocol/server-brave-search\"],\"env\":{\"BRAVE_API_KEY\":\"env:BRAVE_API_KEY\"}}}"
```

工具名以各 Server 为准，且在所有 Connector 间全局唯一。

### 工具目录（启用 / 停用 / 手加 REST）

每个 Connector 拥有一份**工具目录**，落盘在 Store（默认 SQLite）。`GET /v0/tools` 返回目录行——**包含已停用行**——以便设置页列出并重新启用；内存 Registry 只注册 `enabled = true` 的行，因此停用的工具对模型和 invoke 都不可见。

- **显示名 / 说明** — 目录行可带 `title`（只出现在设置页，不进模型 tool list）与 `description`。`PATCH /v0/tools/{name}` 可改 `title` / `description`；人改过的 `description` 带 `description_custom`，再 `PUT` spec 不覆盖。合并时 `title` 始终保留。
- **启用 / 停用** — `PATCH /v0/tools/{name}` 带 `{"enabled": false}`（或 `true`）立即在 Registry 中卸下（或重新注册）该工具，并把标志落盘到目录。SQLite 下停用行与手加行重启后仍在。同一接口也可改行上的 `require_login`。设置页组启停是多次 `PATCH enabled`，不是新接口。
- **设置页树与搜索** — Tools 页按 Connector 与路径前缀折叠，并支持搜索；不新增目录 HTTP 面。
- **手加 REST 工具（仅 OpenAPI）** — `POST /v0/connectors/{id}/tools` 在 `openapi` Connector 上加一行 `extra`，复用该 Connector 的 `base_url`、鉴权、会话身份与 HITL。规格漏写的接口可由此补上。重名返回 `409`。
- **插件工具** — 侧车发现的 `plugin` 行可启停，但**不能**手加或删除，侧车是唯一来源。对 `http` Connector 手加会返回 `400`。
- **删除** — `DELETE /v0/connectors/{id}/tools/{name}` 仅对 `source = extra` 行生效；`spec` / `plugin` 行返回 `400`。
- **再 PUT / 重启** — 同 `id` 再 `PUT` 与已有目录合并（见上文）；Runtime 重启时从磁盘目录恢复，只重新注册启用行。YAML / PUT 可省略目录字段以保持磁盘目录不变。

这个目录开关**不是**：

- **会话登录** — 工具行上的 `require_login` 控制带 `conversation_id` 的 Run 在没有捕获身份时能否调用该工具；它不存凭证。
- **控制面口令** — `control_plane.operator_token` / `admin_token` 挡的是谁能调 `/v0`。目录写操作（`PATCH /v0/tools/{name}`、`POST/DELETE /v0/connectors/{id}/tools`）需要管理员口令；操作员返回 `403`。

### Agent Skills（可选）

Skill 是可选的**配置形态**（不升格为第六抽象）：一份 `SKILL.md`（流程 Markdown）+ 工具名列表。用来收窄（或叠加）模型可见的已启用工具，并把流程正文注入 Run 的 system。

| 主题 | 行为 |
|------|------|
| 落盘 | 内置 `./skills`（`skills.builtin_dir`）与用户 `./data/skills`（`skills.user_dir`）；一级子目录为包 id，内含 `SKILL.md` |
| 上传 / 删除 | 管理员经 `POST /v0/skills` 或「设置 → Skills」上传 `.md` / `.zip`；`DELETE /v0/skills/{id}` 仅删 **user** 包（内置 → `400`）。同 id：用户覆盖内置 |
| 默认激活 | `agent.skills`（YAML / `PUT /v0/agents/{id}`）列出 Run 开始时已激活的包 |
| 渐进激活 | 安装集非空时，模型可见内置工具 `activate_skill`，可在**本 Run** 内扩大激活集 |
| `agent.skills` 为空 | 可见工具 = 目录全部 **enabled**（与引入 Skill 前一致） |
| 求交 | 可见工具 = ∪(已激活 Skill 的 `tools`) ∩ 目录 `enabled`；Skill **不能**启用已停用的目录行 |

开箱试用包在 `examples/skills/ticket-triage`；仅 `demo` / `docker-demo` 扫描该目录。生产 `minimal.yaml` 的 `skills.builtin_dir` 为空，设置页 Skills 初始为空，需自行上传。

这**不是** Cursor 个人编码 Skill 市场，也不保证与上游包（如 `grill-me` / `superpowers`）原样子调度兼容——仅 `SKILL.md` 的 frontmatter + 正文形态尽量可对照。

### 生产一键启动（Go 原生）

`baize start` 使用 `configs/minimal.yaml`：**无演示 Connector、无 mock-ticket、真实 LLM**。须先设置 API Key：

```bash
cp .env.example .env   # 填写 BAIZE_API_KEY
export BAIZE_API_KEY=sk-...   # 或 source .env
go run ./cmd/baize start
# Windows: .\start.cmd  或  .\baize.cmd start  （会读取 .env）
```

可选：复制 `configs/minimal.yaml` → `configs/minimal.local.yaml`（gitignore）覆盖 `llm.base_url` / `model` 等。

若曾跑过 `demo` 且 Tools 里仍有 `ticket-api` / 工单工具，多半是旧库 `./data/baize.db` 里残留的 Connector。`demo` 现已改用 `./data/baize-demo.db`；生产可删一次旧库后重启：

```powershell
Remove-Item -Recurse -Force .\data\baize.db -ErrorAction SilentlyContinue
.\start.cmd
```

启动日志应出现 `baize start: config=configs/minimal.yaml agent=default-agent llm=openai_compatible`，且**不应**有 `persisted connectors restored`（全新库时）。

启动后工具目录为空，需注册 Connector（`PUT /v0/connectors/{id}`）或 MCP Server（`type: mcp`）。SQLite 仍用于 Run / 对话落盘。

### 真实 LLM 与对接业务 API

**不要把密钥写进 YAML。**

1. 生产默认已在 `configs/minimal.yaml` 使用 `openai_compatible`；`baize start` 在缺少 `BAIZE_API_KEY` 时会直接失败并提示。
2. 复制 `.env.example` → `.env`，填写 `BAIZE_API_KEY`（以及可选的控制面 / Connector Token）
3. 在 `configs/minimal.local.yaml` 中增加 `connector` 段，或启动后用 API 注册：

```yaml
connector:
  id: my-api
  type: openapi
  spec: path/to/your/openapi.yaml
  base_url: https://your-api.example.com
  require_approval_mutating: true
  auth:
    mode: static
    static:
      headers:
        Authorization: "Bearer ${BAIZE_CONNECTOR_TOKEN}"
    capture:
      tool_name_glob: "*login*"
      token_json_paths: ["accessToken", "data.accessToken", "data.token"]
      header_template: "Bearer {{token}}"
      default_scheme: "bearer"
```

```bash
go run ./cmd/baize start
# 或：go run ./cmd/baize serve -config configs/minimal.local.yaml
```

试用栈仍用 `go run ./cmd/baize demo`（mock LLM，无需 Key）。

---

## 写操作与身份

变更类工具可通过 `require_approval` 先走人工审批，再真正调用（演示流程见上文「30 秒跑通」）。

### 会话身份

登录类 Tool 成功后，可将 Token **捕获**到按 `conversation_id` 隔离的身份库；同一会话后续调用在已有会话身份时会自动带上解析出的 Bearer（默认按 OpenAPI `securitySchemes` 选型）。

带 `conversation_id` 的 Run（含 `/ui`）**只用**会话身份，不会静默使用 Connector 配置里的默认 Token。配置 Token 可选，仅供**不带** `conversation_id` 的脚本 / curl 使用。

- `/ui` → **设置 → 账号**：查看账号（脱敏）、设默认、退出
- `/ui` → **设置 → Tools**：可标「需要登录」（默认公开，不读 OpenAPI `security`）
- 左栏 **新对话** → 新的 `conversation_id`（`localStorage` 键 `baize.conversation_id`）
- **不带** `conversation_id` 的 Run，默认 HTTP 头由 `connector.auth.mode` 决定：
  - `static` — 注册时展开 `${ENV}`（如 `Bearer ${BAIZE_CONNECTOR_TOKEN}`）
  - `passthrough` — 每次 `POST /v0/runs` 按白名单透传请求头
  - `vault_ref` — 注册时解析 `env:` / `file:` 引用
- 默认捕获匹配 `*login*`，读取 `accessToken` / `data.token` 等（可用 `connector.auth.capture` 覆盖；`tool_name_glob: "__none__"` 关闭）
- `baize start` 与 `PUT /v0/connectors` 都会挂上 Identities / Resolver / Capture（OpenAPI）
- 修改鉴权 / 捕获 YAML 后需重启 Runtime；`PATCH /v0/tools/{name}` 会把 `enabled` / `require_login` 落到目录（SQLite 重启后仍在），并立即在内存 Registry 中注册或卸下该工具。这个开关与对话里登录下游系统（会话身份）、与控制面口令不是同一回事。

```bash
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/identities
```

### 对话记忆

默认 SQLite 驱动下，白泽会把对话消息与捕获的身份写入 `data/baize.db`（可用 `store.sqlite_path` 配置）。Runtime 重启后，按 `conversation_id` 恢复历史轮次与已登录账号。

- `conversation.max_messages`（默认 `40`）控制回喂给 LLM 的最近轮次窗口；更早的消息仍留存数据库备查，但不会进入提示词。配置为 `<=0` 时会在加载时回填为 `40`。
- 「清空聊天」（`DELETE /v0/conversations/{id}/messages`）只删除消息历史，**不会**退出登录；捕获的身份仍在，需通过身份 API 单独删除。消息清空后该对话也会从**左栏列表消失**。
- `GET /v0/conversations` 返回摘要；标题取自首条用户消息，截断到 40 字（不用 LLM 起标题）。
- `data/baize.db` 是运行时产物，请勿提交到 git（默认 `.gitignore` 已忽略 `data/`）。

```bash
# 对话列表（左栏）
curl -s http://127.0.0.1:8080/v0/conversations

# 查看持久化的对话轮次
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages

# 清空聊天但不退出登录
curl -s -X DELETE http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages
```

---

## 部署

白泽是单个静态二进制。可选启动脚本（Windows 无需全局 `baize` 命令）：

- 生产：`.\start.cmd` 或 `.\baize.cmd start`（读取 `.env` 中的 `BAIZE_API_KEY`）
- 试用：`.\demo.cmd` 或 `.\baize.cmd demo`
- POSIX 生产：`./scripts/start.sh`；试用：`./scripts/demo.sh`

### 本机二进制

```bash
# Linux 服务器
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o baize ./cmd/baize
# macOS
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o baize ./cmd/baize
# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o baize.exe ./cmd/baize
```

将二进制拷到目标主机即可，除操作系统外无其它运行时依赖。

### Docker

**生产（仅 Runtime，无演示 HTTP）**：

```bash
export BAIZE_API_KEY=sk-...
docker compose up --build
```

- Runtime / UI：http://127.0.0.1:8080 （`/ui`）
- 使用 `configs/docker-minimal.yaml`；须设置 `BAIZE_API_KEY`

**试用样板栈**（Runtime + 演示 HTTP，mock LLM）：

```bash
docker compose -f docker-compose.demo.yml up --build
```

- Runtime / UI：http://127.0.0.1:8080
- 演示 HTTP：http://127.0.0.1:18080
- 使用 `configs/docker-demo.yaml`；`mock-ticket` 为独立容器

试用 compose 可设置 `BAIZE_CONNECTOR_TOKEN` 为 `dev`，仅供不带会话的脚本；`/ui` 不会用它。不要把密钥写进 YAML。

**MCP + Postgres 试用**（Runtime + 演示库，不含 MCP Server）：

```bash
export BAIZE_API_KEY=sk-...
docker compose -f docker-compose.mcp-demo.yml up --build
```

DBHub 宿主机 `npx` 与 `PUT` 示例见 [MCP Connector](#mcp-connector可选)。

**自定义 YAML 挂载**：

```bash
docker build -t baize:local .
docker run --rm -p 8080:8080 \
  -v /path/to/your.yaml:/app/configs/docker-minimal.yaml \
  -v baize-data:/app/data \
  -e BAIZE_API_KEY \
  baize:local
```

如需覆盖构建时的模块代理：`docker build --build-arg GOPROXY=https://proxy.golang.org,direct .`

---

## Chat UI 构建（可选）

`/ui` 是 React + Vite 单页，仓库已提交 `internal/ui/dist` 预构建产物（`//go:embed`）。修改 `web/chat` 后需 Node 18+ 重新构建：

```bash
cd web/chat
npm ci
npm run build
```

---

## 命令

| 命令 | 说明 |
|------|------|
| `baize start` | 生产默认：`minimal.yaml`；仅 Runtime；**须** `BAIZE_API_KEY`；无演示 Connector |
| `baize demo` | 试用：`demo.yaml`；Runtime + 内嵌演示 HTTP；mock LLM，无需 Key |
| `baize serve -config <path>` | 仅 Runtime，显式指定配置（不校验 API Key） |

---

## 文档

- [架构与插件协议草案](docs/architecture-and-plugin-protocol.md)

---

## 许可证

[MIT](LICENSE)
