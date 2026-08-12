# Baize（白泽）

面向企业遗留 HTTP API 的轻量 Agent Runtime：用 OpenAPI 把接口变成工具，在进程内跑可审计的 ReAct `Run`。

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

### 创建工单

```bash
curl -s -X POST http://127.0.0.1:8080/v0/runs \
  -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"ticket-agent\",\"input\":\"创建一个紧急工单：VPN 挂了\"}"
```

### 查看工单与事件

```bash
curl -s http://127.0.0.1:18080/tickets
curl -s http://127.0.0.1:8080/v0/runs/<run_id>/events
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

- [架构与插件协议草案](docs/architecture-and-plugin-protocol.md)

## 许可证

MIT
