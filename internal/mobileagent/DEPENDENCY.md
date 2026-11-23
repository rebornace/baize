# MOBILE-RT M0 — 依赖审计

| 类别 | 状态 |
|------|------|
| `cmd/baize` | **禁止**；本包与 `cmd/mobilebind` / `mobileagent` 均不导入 |
| 渠道 / MCP / PG / OTel / Redis / MinIO | **不导入** |
| `internal/run` Engine | **不链接**；编排 = 本包内存会话 + ChatOnce / Send 工具循环 |
| `internal/llm` | M0 **不导入**（避开 `store`→pgx）；自含 OpenAI 兼容 HTTP 客户端 |
| gomobile | 绑定公开包 **`./mobileagent`**（再导出本包）；**不能** bind `internal/` |
| 工具依赖 | 重建用 **`gomobile@latest`** / `gobind`（Go 1.26+ 勿用过旧 x/mobile）；脚本 `scripts/build-mobileagent-aar.ps1`、`scripts/build-mobileagent-ios.sh` |

验证：

```text
go list -deps ./internal/mobileagent ./mobileagent | findstr /i "pgx otel redis minio mcp baize/cmd"
# 期望：无输出
```
