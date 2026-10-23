# mcp-sandbox（终端 / 代码执行走 MCP 连接器）

Baize **不**把 shell / 代码执行做成 core tool。需要这类能力时，把沙箱做成
进程外 **MCP 连接器** 接入：工具进 catalog、走 `run.binding` / 审批 / 审计，
与 OpenAPI 连接器同一路径。

本目录只提供接线菜谱，不捆绑具体沙箱实现。

## 推荐形态

| 能力 | 放在哪 | 不要做 |
|------|--------|--------|
| `run_command` / `write_file` / `read_file` | 外部 MCP Server（容器 / 远端 sandbox） | 写进 baize 二进制 |
| 鉴权 | `connector.mcp.headers` 或 stdio 本机信任 | 在 agent system prompt 塞密钥 |
| 审批 | 对写盘 / 执行类工具开 `require_approval` | 旁路 HITL |

## 接线步骤

1. 自备或选用任意 MCP sandbox（stdio 或 Streamable HTTP）。工具名随意，
   只要在 MCP `tools/list` 里暴露，并按需打上 `ReadOnlyHint`。
2. 复制 [`connector.mcp.example.yaml`](connector.mcp.example.yaml) 到本地
   `configs/*.local.yaml`（该文件被 gitignore，勿提交真实 URL / token）。
3. 启动 baize 后在设置页确认连接器工具出现；对破坏性工具勾选审批。
4. 对话里让模型调用沙箱工具；Run 事件与 `run.binding` 应能复盘到具体
   MCP tool 名与参数摘要。

## Stdio 示例（本机调试）

```yaml
connector:
  id: sandbox-stdio
  type: mcp
  mcp:
    transport: stdio
    command: your-sandbox-mcp
    args: ["--root", "/tmp/sandbox"]
```

## HTTP 示例（远端沙箱）

见 `connector.mcp.example.yaml`。Cloud / 内网 endpoint 用环境变量注入
`${SANDBOX_MCP_URL}` / `${SANDBOX_TOKEN}`，不要写进仓库。

## 与 Hermes 的差异

Hermes 内置多种终端后端。Baize 刻意保持「旁挂 + 审计」：执行面在连接器，
策略面在 baize（审批、登录、DP-2a 收窄、binding）。
