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

## Chat UI 构建（可选）

仓库已提交 `internal/ui/dist` 预构建产物，干净 clone 后即可 `go build` / `go test`（`//go:embed`）。若修改 `web/chat`，需 Node 18+ 重新构建：

```bash
cd web/chat
npm ci --registry=https://registry.npmmirror.com
npm run build
# 产物输出到 internal/ui/dist，随后可 go test ./...
```

## 切换真实 LLM（openai_compatible）

编辑 `configs/demo.yaml`：

```yaml
llm:
  provider: openai_compatible
  base_url: https://api.openai.com/v1
  model: gpt-4o-mini
  # api_key_env: BAIZE_API_KEY   # 默认读取此环境变量
```

然后设置密钥并启动：

```bash
# Windows PowerShell
$env:BAIZE_API_KEY="sk-..."
go run ./cmd/baize demo
```

也可用 `go run ./cmd/baize serve -config configs/demo.yaml` 仅启动 Runtime（需自行提供 connector 指向的后端）。

## 文档

- [架构与插件协议草案](docs/architecture-and-plugin-protocol.md)（含 [文档第二页：HITL 拟稿 → resume → 写回](docs/architecture-and-plugin-protocol.md#7-首个验收故事)）
- [Demo B 设计规格（Chat + HITL）](docs/superpowers/specs/2026-08-12-baize-demo-b-design.md)

## 许可证

MIT
