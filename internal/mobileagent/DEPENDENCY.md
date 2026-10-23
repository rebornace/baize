# MOBILE-RT M0 — 依赖审计

| 类别 | 状态 |
|------|------|
| `cmd/baize` | **禁止**；本包与 `cmd/mobilebind` / `mobileagent` 均不导入 |
| 渠道 / MCP / PG / OTel / Redis / MinIO | **不导入** |
| `internal/run` Engine | M0 **不链接**；编排 = 本包内存会话 + ChatOnce |
| `internal/llm` | M0 **不导入**（避开 `store`→pgx）；自含 OpenAI 兼容 HTTP 客户端 |
| gomobile | 绑定公开包 **`./mobileagent`**（再导出本包）；**不能** bind `internal/` |
| 工具依赖 | `go.mod` `tool`：`golang.org/x/mobile/cmd/gobind`（重建 AAR 用） |

验证：

```text
go list -deps ./internal/mobileagent ./mobileagent | findstr /i "pgx otel redis minio mcp baize/cmd"
# 期望：无输出
```
