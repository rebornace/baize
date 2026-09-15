# CLEAN-GATES golangci 全量基线

> 日期：2026-09-15  
> 工具：golangci-lint **v2.4.0**（与 CI 一致）  
> 命令：`golangci-lint run ./... --timeout=5m`  
> 分支：`feat/clean-gates` @ `4aae6f0`  
> 计划：[`../plans/2026-09-15-clean-gates.md`](../plans/2026-09-15-clean-gates.md) 任务 1  
> 对照 AUDIT §6：约 46 issues（errcheck 20 / staticcheck 22 / ineffassign 2 / unused 2）

## 汇总

| 项 | 值 |
|----|-----|
| 总 issues | **46** |
| 退出码 | 1 |
| `.superpowers` 噪声 | **无**（未命中） |
| 与 AUDIT 一致 | **是** |

### 按 linter

| Linter | Count | 建议修复批次（任务 2） |
|--------|------:|------------------------|
| unused | 2 | 批次 1 |
| ineffassign | 2 | 批次 1 |
| staticcheck | 22 | 批次 2 |
| errcheck | 20 | 批次 3 |
| **合计** | **46** | |

### Top 10 文件（按 issue 数）

| # | Count | File |
|--:|------:|------|
| 1 | 6 | `internal/conversation/summary_rolling_test.go` |
| 2 | 4 | `internal/attach/image.go` |
| 3 | 4 | `internal/run/compact_engine_test.go` |
| 4 | 3 | `internal/middleware/reconciler_test.go` |
| 5 | 3 | `internal/api/server_sse_poll_test.go` |
| 6 | 3 | `internal/llm/switch_test.go` |
| 7 | 2 | `internal/plugincallback/limiter_test.go` |
| 8 | 1 | （其余各 1；见下方完整清单） |

其余 1 条/文件：`examples/mock-ticket/server_test.go`、`examples/mcp-mock/main.go`、`internal/middleware/worker_test.go`、`internal/skill/loginmanage/sync_test.go`、`internal/store/sqlite_connectors.go`、`internal/store/store_test.go`、`internal/channel/runtime.go`、`internal/connector/openapi/loader.go`、`internal/api/server_inbox.go`、`internal/channelmedia/channelmedia.go`、`internal/config/validate_start.go`、`internal/connector/mcp/client_http_test.go`、`internal/connector/register_mcp_oauth_test.go`、`internal/connector/specimport/fetch_test.go`、`internal/conversation/sql_dialect.go`、`internal/eventbus/store.go`、`internal/inbox/ratelimit_test.go`、`internal/runtimecfg/persist.go`、`tests/integration/weixin_adapter_test.go`、`internal/connector/httpplugin/register_opts.go`、`internal/inbox/verify.go`。

---

## 修复清单（按批次）

### 批次 1 — unused（2）

| File:Line | Message |
|-----------|---------|
| `internal/connector/httpplugin/register_opts.go:43` | const `defaultCallbackTTL` is unused |
| `internal/inbox/verify.go:10` | const `signaturePrefix` is unused |

### 批次 1 — ineffassign（2）

| File:Line | Message |
|-----------|---------|
| `internal/channel/runtime.go:433` | ineffectual assignment to `llmText` |
| `internal/connector/openapi/loader.go:145` | ineffectual assignment to `bodyKind` |

### 批次 2 — staticcheck（22）

| File:Line | Code | Message |
|-----------|------|---------|
| `examples/mcp-mock/main.go:27` | S1016 | convert `echoArgs` → `echoOutput` |
| `internal/api/server_inbox.go:106` | QF1002 | tagged switch on `action` |
| `internal/api/server_sse_poll_test.go:60` | QF1006 | lift into loop condition |
| `internal/api/server_sse_poll_test.go:77` | QF1006 | lift into loop condition |
| `internal/api/server_sse_poll_test.go:130` | QF1006 | lift into loop condition |
| `internal/attach/image.go:9` | ST1019 | `image/jpeg` imported more than once |
| `internal/attach/image.go:10` | ST1019 | `image/png` imported more than once |
| `internal/attach/image.go:14` | ST1019 | related: other import of `image/jpeg` |
| `internal/attach/image.go:15` | ST1019 | related: other import of `image/png` |
| `internal/channelmedia/channelmedia.go:173` | QF1001 | apply De Morgan's law |
| `internal/config/validate_start.go:23` | ST1005 | error strings should not end with punctuation/newlines |
| `internal/connector/mcp/client_http_test.go:28` | S1016 | convert `httpEchoArgs` → `httpEchoOutput` |
| `internal/connector/register_mcp_oauth_test.go:69` | S1016 | convert `mcpOAuthEchoArgs` → `mcpOAuthEchoOutput` |
| `internal/connector/specimport/fetch_test.go:36` | S1021 | merge var declaration with assignment |
| `internal/conversation/sql_dialect.go:16` | QF1003 | tagged switch on `dialect` |
| `internal/eventbus/store.go:19` | QF1008 | remove embedded field `Store` from selector |
| `internal/inbox/ratelimit_test.go:12` | SA4000 | identical expressions on `\|\|` |
| `internal/middleware/reconciler_test.go:22` | S1011 | replace loop with `append(out, s.runs...)` |
| `internal/plugincallback/limiter_test.go:24` | SA4000 | identical expressions on `\|\|` |
| `internal/plugincallback/limiter_test.go:30` | SA4000 | identical expressions on `\|\|` |
| `internal/runtimecfg/persist.go:254` | S1016 | convert `OperatorInput` → `operatorEntry` |
| `tests/integration/weixin_adapter_test.go:39` | QF1002 | tagged switch on `r.URL.Path` |

### 批次 3 — errcheck（20）

| File:Line | Message |
|-----------|---------|
| `examples/mock-ticket/server_test.go:28` | `(*json.Decoder).Decode` unchecked |
| `internal/conversation/summary_rolling_test.go:10` | `UpsertRollingSummary` unchecked |
| `internal/conversation/summary_rolling_test.go:21` | `UpsertRollingSummary` unchecked |
| `internal/conversation/summary_rolling_test.go:32` | `Append` unchecked |
| `internal/conversation/summary_rolling_test.go:42` | `Append` unchecked |
| `internal/conversation/summary_rolling_test.go:43` | `UpsertRollingSummary` unchecked |
| `internal/conversation/summary_rolling_test.go:56` | `Append` unchecked |
| `internal/llm/switch_test.go:115` | `sw.Chat` unchecked |
| `internal/llm/switch_test.go:116` | `sw.Chat` unchecked |
| `internal/llm/switch_test.go:120` | `sw.Chat` unchecked |
| `internal/middleware/reconciler_test.go:36` | `defer mw.Close()` unchecked |
| `internal/middleware/reconciler_test.go:75` | `defer mw.Close()` unchecked |
| `internal/middleware/worker_test.go:52` | `defer mw.Close()` unchecked |
| `internal/run/compact_engine_test.go:26` | `ms.Append` unchecked |
| `internal/run/compact_engine_test.go:28` | `UpsertRollingSummary` unchecked |
| `internal/run/compact_engine_test.go:66` | `ms.Append` unchecked |
| `internal/run/compact_engine_test.go:78` | `ms.Append` unchecked |
| `internal/skill/loginmanage/sync_test.go:89` | `DeleteConnector` unchecked |
| `internal/store/sqlite_connectors.go:154` | `defer tx.Rollback()` unchecked |
| `internal/store/store_test.go:24` | `AppendEvent` unchecked |

**errcheck 备注：** 多数在 `*_test.go`；生产路径仅 `sqlite_connectors.go` 的 `tx.Rollback`。defer Close/Rollback 按计划允许显式 `_ =` 或带注释的惯用处理，禁止无注释大范围 `//nolint`。

---

## 原始日志

未提交。本机 tee：`%TEMP%\golangci-gates.txt`（仅本地复现用）。
