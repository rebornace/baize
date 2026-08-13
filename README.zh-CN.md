# Baize（白泽）

[![Go](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License](https://img.shields.io/badge/License-MIT-green.svg)](LICENSE)

[English](README.md) | **中文**

**旁挂任意系统的可插拔 AI Agent —— 接上就能用，卸下不留痕。**

白泽是一个极轻的 Agent Runtime：不改业务代码，只旁挂在现有 API 旁边。导入 OpenAPI，接口自动变成工具；进程内跑可审计的 ReAct 循环，写操作可走人工审批。无论系统多老、多杂，都能以最小代价接上 AI；不绑环境、不绑平台，用完即走。

> 侧车，不是聊天机器人：主路径是 **OpenAPI → Tools → HTTP**，需要时叠加 HITL 审批与会话级凭证。

---

## 为什么需要白泽

| 痛点 | 白泽做法 |
|------|----------|
| 已有 HTTP / OpenAPI，缺 Agent 层 | 注册 Connector，operation 自动变成 Tool |
| LLM 直接写接口风险高 | 变更类工具先 HITL，再真正调用 |
| Agent 行为不透明 | 每次执行都是带事件流的 `Run` |
| 多账号 / 管理端与用户端 Token | 会话身份库 + 可插拔鉴权解析（默认跟 OpenAPI security） |

### 五个概念

| 概念 | 含义 |
|------|------|
| **Runtime** | 承载 Agent、Tool、存储与控制面 HTTP 的进程 |
| **Agent** | 系统提示词 + LLM，驱动一次 Run |
| **Tool** | 可调用能力（通常对应一个 OpenAPI operation） |
| **Connector** | Spec + `base_url`（+ 鉴权）到 Tool 注册表的绑定 |
| **Run** | 一次执行：输入 → ReAct → 输出 / 待审批 / 失败 |

---

## 快速开始

**环境要求：** Go 1.22+

```bash
git clone https://github.com/rebornace/baize.git
cd baize
go run ./cmd/baize start
```

**Windows 一键启动：**

```powershell
.\start.cmd
```

会按需设置 `GOPROXY`，并执行 `go run ./cmd/baize start`。

默认样板：

- Runtime：`http://127.0.0.1:8080`
- Mock 工单 API：`http://127.0.0.1:18080`
- LLM：内置 `mock`（无需 Key）
- Chat UI：http://127.0.0.1:8080/ui

若端口被占用，先结束旧的 `baize` 进程再启动。

### 创建工单（curl）

`POST /v0/runs` 为异步，立即返回 `run_id`。默认 `create_ticket` 需审批，状态为 `waiting_human`。

```bash
curl -s -X POST http://127.0.0.1:8080/v0/runs \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"ticket-agent\",\"input\":\"创建一个紧急工单：VPN 挂了\",\"conversation_id\":\"conv_example_1\"}"
```

```bash
curl -s http://127.0.0.1:8080/v0/runs/<run_id>
```

### HITL 审批

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
```

---

## 平台接入（OpenAPI）

主路径：把遗留 OpenAPI 服务注册成 Tools。

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

> 说明：`PUT /v0/connectors` 目前不会自动挂上会话 Identities / Resolver / Capture。`baize start` 启动路径会为配置中的 Connector 接线；热更新鉴权会话能力后续增强。

---

## 会话身份

登录类 Tool 成功后，可将 Token **捕获**到按 `conversation_id` 隔离的身份库；同一会话后续受保护调用会自动带上解析出的 Bearer（默认按 OpenAPI `securitySchemes` 选型）。

- Chat UI 侧栏：查看账号（脱敏）、设默认、退出
- 「新对话」→ 新的 `conversation_id`
- `bearer_env` 只是**启动兜底**，不是唯一身份来源
- 默认捕获匹配 `*login*`，读取 `accessToken` / `data.token` 等（可用 `connector.auth.capture` 覆盖；`tool_name_glob: "__none__"` 关闭）
- 本地配置若只写了 `bearer_env`，`baize start` 会自动套用捕获默认值
- 修改鉴权 / 捕获配置后需重启 Runtime

```bash
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/identities
```

---

## 对话记忆

默认 SQLite 驱动下，白泽会把对话消息与捕获的身份写入 `data/baize.db`（可用 `store.sqlite_path` 配置）。Runtime 重启后，按 `conversation_id` 恢复历史轮次与已登录账号。

- `conversation.max_messages`（默认 `40`）控制回喂给 LLM 的最近轮次窗口；更早的消息仍留存数据库备查，但不会进入提示词。配置为 `<=0` 时会在加载时回填为 `40`。
- 「清空聊天」（`DELETE /v0/conversations/{id}/messages`）只删除消息历史，**不会**退出登录；捕获的身份仍在，需通过身份 API 单独删除。
- `data/baize.db` 是运行时产物，请勿提交到 git（默认 `.gitignore` 已忽略 `data/`）。

```bash
# 查看持久化的对话轮次
curl -s http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages

# 清空聊天但不退出登录
curl -s -X DELETE http://127.0.0.1:8080/v0/conversations/<conversation_id>/messages
```

---

## 真实 LLM 与真实后端

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
    bearer_env: BAIZE_CONNECTOR_TOKEN
    capture:
      tool_name_glob: "*login*"
      token_json_paths: ["accessToken", "data.accessToken", "data.token"]
      header_template: "Bearer {{token}}"
      default_scheme: "bearer"

mock_ticket:
  listen: "off"   # 对接真实后端时关闭 mock-ticket
```

```bash
go run ./cmd/baize start
# 或：go run ./cmd/baize serve -config configs/default.local.yaml
```

---

## Chat UI 构建（可选）

仓库已提交 `internal/ui/dist` 预构建产物（`//go:embed`）。修改 `web/chat` 后需 Node 18+ 重新构建：

```bash
cd web/chat
npm ci
npm run build
```

---

## 命令

| 命令 | 说明 |
|------|------|
| `baize start` | 优先 `default.local.yaml`，否则 `default.yaml`；启动 Runtime（除非 `mock_ticket.listen: off`，否则同时起 mock 工单） |
| `baize serve -config <path>` | 仅 Runtime，显式指定配置 |

---

## 文档

- [架构与插件协议草案](docs/architecture-and-plugin-protocol.md)

---

## 许可证

[MIT](LICENSE)
