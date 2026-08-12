# Baize（白泽）

面向企业遗留 HTTP API 的轻量 Agent Runtime：用 OpenAPI 把接口变成工具，在进程内跑可审计的 ReAct `Run`；写类工具可经 HITL 审批；运营入口为嵌入的 Chat UI（`/ui`）。

## 快速开始

要求：Go 1.22+

若默认模块代理（`proxy.golang.org`）无法访问，先设置：

```powershell
$env:GOPROXY = "https://goproxy.cn,direct"
$env:GOSUMDB = "sum.golang.google.cn"
```

```bash
go run ./cmd/baize demo
```

若提示端口占用，先结束占用 `:8080` / `:18080` 的旧 `baize` 进程后再启动。
默认会在本机拉起 mock 工单服务（`:18080`）与 Runtime（`:8080`），LLM 使用内置 `mock`（无需 API Key）。

启动后打开浏览器：

```text
http://127.0.0.1:8080/ui
```

### 创建工单（curl）

`POST /v0/runs` 为异步：立即返回 `run_id`；`create_ticket` 默认需审批，状态会进入 `waiting_human`。

```bash
curl -s -X POST http://127.0.0.1:8080/v0/runs \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"ticket-agent\",\"input\":\"创建一个紧急工单：VPN 挂了\"}"
```

轮询状态：

```bash
curl -s http://127.0.0.1:8080/v0/runs/<run_id>
```

### HITL 审批

批准后才会真正调用 mock-ticket；驳回则 Run `failed`，不会新建工单。

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

默认 `configs/demo.yaml` 使用 SQLite 持久化；重启 Runtime 后，对仍为 `waiting_human` 的 Run 可继续 `resume`。

### 查看工单与事件

```bash
curl -s http://127.0.0.1:18080/tickets
curl -s http://127.0.0.1:8080/v0/runs/<run_id>/events
```

## 平台接入（OpenAPI）

主路径：用 OpenAPI Spec 把遗留 HTTP API 注册为 Tools。无 Swagger 的侧车协议见后续里程碑；鉴权深做不在本里程碑。

1. **准备** OpenAPI 文件，以及 Runtime 可到达的后端 `base_url`。
2. **注册/替换 Connector：**

```bash
curl -s -X PUT http://127.0.0.1:8080/v0/connectors/ticket-api \
  -H "Content-Type: application/json" \
  -d "{\"type\":\"openapi\",\"spec\":\"examples/mock-ticket/openapi.yaml\",\"base_url\":\"http://127.0.0.1:18080\",\"require_approval\":[\"create_ticket\"]}"
```

3. **核对 Tools：**

```bash
curl -s http://127.0.0.1:8080/v0/tools
```

应看到带 `method` / `path` / `operation_id` / `connector_id` 的清单。

4. **跑一次：** `POST /v0/runs`（见上文）或打开 `http://127.0.0.1:8080/ui`。

同 `id` 再 `PUT` 会按新 Spec **整表替换**该 Connector 下的 Tools；坏 Spec 返回 `400`，不污染已有 Registry。

注意：`PUT /v0/connectors` 当前不会自动挂上会话 Identities/Resolver/Capture（`baize demo` 启动路径会挂；热更新 connector 需后续增强）。

## 会话身份

对话内登录成功后，凭证按 `conversation_id` 记在会话身份库；同一会话后续受保护调用会自动带上捕获的 Bearer。Chat UI 侧栏可查看已登录账号（脱敏）、设默认与退出；新对话换新 `conversation_id`。`bearer_env` 仅作无会话捕获时的启动兜底，不是唯一身份来源。

## Chat UI 构建（可选）

仓库已提交 `internal/ui/dist` 预构建产物，干净 clone 后即可 `go build` / `go test`（`//go:embed`）。若修改 `web/chat`，需 Node 18+ 重新构建：

```bash
cd web/chat
npm ci --registry=https://registry.npmmirror.com
npm run build
# 产物输出到 internal/ui/dist，随后可 go test ./...
```

## 切换真实 LLM（openai_compatible）

推荐本地调试用 DeepSeek Flash（便宜），且**不要把 Key 写进 YAML**：

1. 复制 `configs/demo.yaml` → `configs/demo.local.yaml`（若存在，`baize demo` 自动优先读它；该文件已 gitignore）
2. 复制 `.env.example` → `.env`，填入 `BAIZE_API_KEY`
3. 在 `demo.local.yaml` 中例如：

```yaml
llm:
  provider: openai_compatible
  base_url: https://api.deepseek.com
  model: deepseek-v4-flash
  disable_thinking: true   # 关掉默认 thinking，少烧 token
  api_key_env: BAIZE_API_KEY
```

```bash
go run ./cmd/baize demo
```

也可用环境变量临时覆盖（不写 `.env`）：

```powershell
$env:BAIZE_API_KEY="sk-..."
go run ./cmd/baize demo
```

也可用 `go run ./cmd/baize serve -config configs/demo.yaml` 仅启动 Runtime（需自行提供 connector 指向的后端）。默认 CI / 无 Key 仍用 `mock`。

## 文档

- [架构与插件协议草案](docs/architecture-and-plugin-protocol.md)（含 [文档第二页：HITL 拟稿 → resume → 写回](docs/architecture-and-plugin-protocol.md#7-首个验收故事)）
- [Demo B 设计规格（Chat + HITL）](docs/superpowers/specs/2026-08-12-baize-demo-b-design.md)

## 许可证

MIT
