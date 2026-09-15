# 入门：构建、测试与贡献

## 环境

| 组件 | 版本 | 说明 |
|------|------|------|
| Go | **1.25.0** | 与 [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) 一致 |
| Node.js | 22 | 仅 `web/chat` 前端 CI / 本地校验需要 |

## Go

在仓库根目录：

```bash
go vet ./...
test -z "$(gofmt -l .)"   # Windows PowerShell 可用 gofmt -l . 检查是否有输出
go build ./...
go test -count=1 ./...
```

CI 还会跑 `golangci-lint`（`only-new-issues`）。本地可选安装同版本后执行。

集成测试若依赖 Postgres，可设置 `BAIZE_TEST_PG_DSN`（CI 已注入）。

## 前端（`web/chat`）

```bash
cd web/chat
npm ci
npm run lint
npm test
npx tsc --noEmit
```

## 本地试用

仓库根目录：

- Windows：`.\demo.cmd`
- Unix：`./scripts/demo.sh`

生产向启动见根 README 的 `start` / `baize start`（需 `BAIZE_API_KEY`）。更多部署见 [deployment](./deployment.md)。

## 目录地图（贡献者）

| 路径 | 说明 |
|------|------|
| `internal/api/` | HTTP 控制面；`server.go` 路由注册，handlers 按域分文件（含 `server_runs.go` / `server_runs_exec.go`） |
| `web/chat/src/api/` | 浏览器客户端；`api.ts` 为 barrel；settings 见 `api/settings/*` |
| `web/chat/src/pages/chat/` | Chat 页 hooks/子组件；入口 `pages/ChatPage.tsx` |
| `web/chat/src/pages/` | Settings 入口页；Tools/Models/MCP Export 等已拆子目录（如 `pages/tools/`） |
| `internal/run/` | Run 引擎；`engine.go` 入口/循环，步进/流式等见 `engine_*.go` |
| `internal/store/` | 持久化；`sqlite.go` 打开/迁移，实体 SQL 见 `sqlite_*.go` |

## 贡献

1. 从 `main` 开分支，提 PR 合入 `main`。
2. 保持 `gofmt` / `go vet` / `go test` 与前端 lint、test、`tsc` 与 CI 一致。
3. 设计讨论通过 Issue / PR；不要在公开文档里堆未落地的路线图。
