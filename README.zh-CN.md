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

## 30 秒跑通

**环境要求：** Go 1.22+（无需 C 编译器；SQLite 为纯 Go）

```bash
git clone https://github.com/rebornace/baize.git
cd baize
go run ./cmd/baize start
```

仓库会同时拉起一个 **mock HTTP 演示服务**，方便你点 UI / 打 curl；不是产品本身。

默认样板：

- Runtime：`http://127.0.0.1:8080`（`/ui`）
- 演示 HTTP：`http://127.0.0.1:18080`
- LLM：内置 `mock`（无需 Key）

若端口被占用，先结束旧的 `baize` 进程再启动。

### 操作员界面（`/ui`）

打开 `http://127.0.0.1:8080/ui`。若配置了控制面口令，打开 `/ui` 先解锁；操作员只能进账号，改 Tools 需要管理员口令。

- 左侧：对话列表 + **新对话**；左下角 **设置**（操作员显示「账号」）
- 主区：消息流；写工具以**卡片**展示（名称 + 状态），展开可见参数 / 结果
- `waiting_human`：在卡片上 **批准 / 驳回**（没有底部大横幅）
- 设置：Tools「需要登录」仅管理员可改；账号页操作员可用；MCP / 插件为「即将接入」空状态（不填假配置）
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

默认 `configs/default.yaml` 使用 SQLite；Runtime 重启后，仍可对 `waiting_human` 的 Run 继续 `resume`。

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

同 `id` 再 `PUT` 会**整表替换**该 Connector 下的 Tools；坏 Spec 返回 `400`，不污染已有 Registry。

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

HITL 仍使用 `require_approval`。默认 `baize start` 仍是仓库自带的演示 OpenAPI Connector。

### 真实 LLM

**不要把密钥写进 YAML。**

> 若你之前使用 `configs/demo.local.yaml`，请改名为 `configs/default.local.yaml`，并把 `demo.ticket_listen` 改为 `mock_ticket.listen`。

1. 复制 `configs/default.yaml` → `configs/default.local.yaml`（已 gitignore；`baize start` 优先读取）
2. 复制 `.env.example` → `.env`，填写 `BAIZE_API_KEY`（以及可选的 `BAIZE_CONNECTOR_TOKEN`）
3. `default.local.yaml` 示例片段：

```yaml
llm:
  provider: openai_compatible
  base_url: https://api.deepseek.com
  model: deepseek-v4-flash
  disable_thinking: true
  api_key_env: BAIZE_API_KEY

connector:
  # 指向你的 OpenAPI 与 base_url
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

mock_ticket:
  listen: "off"   # 对接你的服务时关闭演示 HTTP
```

```bash
go run ./cmd/baize start
# 或：go run ./cmd/baize serve -config configs/default.local.yaml
```

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
- 修改鉴权 / 捕获 YAML 后需重启 Runtime；`PATCH /v0/tools/{name}` 仅进程内临时生效；重启或再次 PUT 后以 YAML / PUT 为准

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

白泽是单个静态二进制。可选启动脚本（未设置 `GOPROXY` 时会写入国内代理）：

- POSIX：`./scripts/start.sh`
- Windows：`.\start.cmd`

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

**试用样板栈**（Runtime + 演示 HTTP，mock LLM）：

```bash
docker compose up --build
```

- Runtime / UI：http://127.0.0.1:8080 （`/ui`）
- 演示 HTTP：http://127.0.0.1:18080

须同时启动**两个**服务。`configs/docker.yaml` 将 `base_url` 指向主机名 `mock-ticket`；只起 Runtime 时，打到演示 HTTP 的工具会连不上。

试用 compose 仍默认设置 `BAIZE_CONNECTOR_TOKEN` 为 `dev`，仅供不带会话的脚本可选使用；`/ui` 不会用它。不要把密钥写进 YAML。

**生产侧车**（你的 HTTP 服务，无演示 HTTP）：

```bash
docker build -t baize:local .
docker run --rm -p 8080:8080 \
  -v /path/to/your.yaml:/app/configs/docker.yaml \
  -v baize-data:/app/data \
  -e BAIZE_API_KEY \
  baize:local
```

在 `your.yaml` 中设置 `mock_ticket.listen: off`，并将 `connector.base_url` 指向你的 HTTP 服务。不要把本仓库的 compose 文件当作生产编排。

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
| `baize start` | 优先 `default.local.yaml`，否则 `default.yaml`；启动 Runtime（除非 `mock_ticket.listen: off`，否则同时起演示 HTTP 服务） |
| `baize serve -config <path>` | 仅 Runtime，显式指定配置 |

---

## 文档

- [架构与插件协议草案](docs/architecture-and-plugin-protocol.md)

---

## 许可证

[MIT](LICENSE)
