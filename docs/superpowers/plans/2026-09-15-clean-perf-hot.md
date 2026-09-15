# CLEAN-PERF-HOT 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** 对 AUDIT §7 四条热点建立可复现本地探针与基线笔记；仅当中位数改善 ≥20%（或绝对延迟对体感有明确价值）时做小优化；有证据才回填 README 中/英「性能说明」。

**架构：** 以 Go `Benchmark*` / 可重复 `go test -bench` 为主（CI 默认不跑 bench）；可选 `scripts/perf-probes.ps1` 一键跑四探针并打印摘要。私有笔记存环境与数字；公开仓只有脚本、开发者文档、README。无证据不改业务热路径。

**技术栈：** Go 1.25、`testing.B`、PowerShell。分支：`feat/clean-perf-hot`。Windows：`git commit -m "..."`。禁止 `move_agent_to_root`。

规格：[`../specs/2026-09-15-clean-perf-hot-design.md`](../specs/2026-09-15-clean-perf-hot-design.md)  
父规格 §4.5：[`../specs/2026-09-15-oss-quality-cleanup-design.md`](../specs/2026-09-15-oss-quality-cleanup-design.md)  
候选：[`../notes/2026-09-15-clean-audit.md`](../notes/2026-09-15-clean-audit.md) §7  
前置：GATES 已交付

**硬约束：**

- 同机同命令重复 **≥3** 次取中位数；优化门槛相对 **≥20%**（或笔记论证绝对价值）  
- README **禁止**竞品对比；只声称本地探针环境  
- 渠道出站探针 **禁止**打真实外网（沿用 `httptest` / 内存 store fixture）  
- 不夹带 STRUCT/P2/GATES 无关重构  

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/api/perf_stream_bench_test.go` | 探针 A：SSE stream 首事件/多事件回放计时（`Benchmark*`） |
| `internal/store/perf_messages_bench_test.go` | 探针 B：N 条消息 `ListMessages`（memory 与/或 sqlite temp） |
| `internal/blob/memory/perf_bench_test.go` | 探针 C：memory Put/Get 固定载荷 |
| `internal/blob/file/perf_bench_test.go` | 探针 C：file 驱动 Put/Get（`t.TempDir`） |
| `internal/api/perf_outbox_bench_test.go` | 探针 D：outbound-deliveries 列表（种子 K 行） |
| `scripts/perf-probes.ps1` | 一键跑四探针 `-bench` ×3，打印摘要 |
| `docs/developers/performance.md` | 如何复现（公开） |
| `docs/developers/README.md` | 链到 performance |
| `docs/superpowers/notes/2026-09-15-clean-perf-hot.md` | 基线/结论（私有） |
| `README.md` / `README.zh-CN.md` | 性能说明回填 |
| 账本 / 父规格头 | PERF-HOT 已交付；CLEAN 收口 |

**阈值常量（写入笔记与 performance.md）：** 改善中位数 ≥20% 才开优化任务；否则「已测、保持现状」。

---

### 任务 1：脚手架 — 文档骨架 + 笔记壳 + 脚本壳

**文件：**  
- 创建：`docs/developers/performance.md`  
- 创建：`docs/superpowers/notes/2026-09-15-clean-perf-hot.md`  
- 创建：`scripts/perf-probes.ps1`（可先调空 bench 名，任务 2–5 补齐）  
- 修改：`docs/developers/README.md` 加一行链接  

- [ ] **步骤 1：写 `performance.md`**

内容须含：

```markdown
# 性能探针（本地）

环境：本机 Go（`go version`），非生产 SLA。

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
.\scripts\perf-probes.ps1
```

或单独：

```powershell
go test ./internal/api/ -bench=BenchmarkPerfStream -benchtime=50x -count=3
go test ./internal/store/ -bench=BenchmarkPerfListMessages -benchtime=50x -count=3
go test ./internal/blob/memory/ ./internal/blob/file/ -bench=BenchmarkPerfBlob -benchtime=50x -count=3
go test ./internal/api/ -bench=BenchmarkPerfOutboxList -benchtime=50x -count=3
```

阈值：优化须相对中位数改善 ≥20%，否则保持现状。禁止竞品对比数字。
```

- [ ] **步骤 2：笔记壳**

`2026-09-15-clean-perf-hot.md` 含：日期、`go version`、OS、机器摘要栏（可空待填）、四探针结果表（空）、结论栏、优化决定栏。

- [ ] **步骤 3：脚本壳**

`scripts/perf-probes.ps1`：设 PATH、依次跑上列四条 `go test -bench`、`$LASTEXITCODE` 非 0 则 exit；输出写 `$env:TEMP\baize-perf-probes.txt` 并 `Write-Host` 路径。

- [ ] **步骤 4：README 开发者索引**

在 `docs/developers/README.md` 表格加 `[performance](./performance.md)`。

- [ ] **步骤 5：分支与 Commit**

```powershell
git checkout -b feat/clean-perf-hot
git add docs/developers/performance.md docs/developers/README.md docs/superpowers/notes/2026-09-15-clean-perf-hot.md scripts/perf-probes.ps1
git commit -m "docs: CLEAN-PERF-HOT 探针脚手架与开发者文档"
```

---

### 任务 2：探针 A — Run SSE stream

**文件：**  
- 创建：`internal/api/perf_stream_bench_test.go`  
- 参考：`internal/api/server_sse_poll_test.go`（`flushRecorder`、`NewServer`、memory store、Hub）

- [ ] **步骤 1：实现 `BenchmarkPerfStreamReplay`**

同包 `api` 测试：创建 run，预置 N=100 个 `AppendEvent`（类型可轮换 `llm.message`），然后对每次 `b.N`：`httptest` GET `/v0/runs/{id}/stream?after=-1`，读到 body 含最后一事件或连接由 handler 在 replay 后的行为稳定结束为止。

要点：

- 复用 `flushRecorder`（可同文件复制小类型，避免导出）  
- `b.ReportAllocs()`  
- 不要依赖真实 LLM；只测 store 回放 + SSE 写出  

示意骨架：

```go
func BenchmarkPerfStreamReplay(b *testing.B) {
	mem := store.NewMemory()
	srv := NewServer(mem, tool.NewRegistry(), &gateFakeRunner{store: mem})
	// ... create run, append 100 events ...
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/v0/runs/"+run.ID+"/stream?after=-1", nil)
		rr := &flushRecorder{ResponseRecorder: httptest.NewRecorder()}
		// cancel shortly after body contains expected marker, or use short context
		srv.Handler().ServeHTTP(rr, req)
	}
}
```

若 live loop 不退出：用 `context.WithTimeout`（如 2s）或只测「首屏 replay」路径——以现有 handler 行为为准，在笔记写明测量的是哪一段。

- [ ] **步骤 2：跑通**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
go test ./internal/api/ -bench=BenchmarkPerfStream -benchtime=20x -count=1
```

预期：有 ns/op 输出、PASS。

- [ ] **步骤 3：Commit** `test: 增加 Run SSE stream 性能探针`

---

### 任务 3：探针 B — ListMessages

**文件：** 创建 `internal/store/perf_messages_bench_test.go`

- [ ] **步骤 1：`BenchmarkPerfListMessages`**

使用 `store.NewMemory()`（或 sqlite temp 若 ListMessages 实现更贴近生产——优先测**生产默认路径**：若 demo 用 sqlite，用 `t.TempDir` 打开 sqlite；若 API 测试多用 memory，memory 可接受，**笔记标明驱动**）。

种子：同一 conversation **N=500** 条 `Append`（role user/assistant 交替），`b.N` 内反复 `ListMessages(convID, opts)`（与生产 list API 相同的 limit/默认 opts；对照 `handleListMessages` 所用 store 方法签名）。

```go
func BenchmarkPerfListMessages(b *testing.B) {
	// seed 500 messages once outside timer
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := st.ListMessages(ctx, convID, store.ListMessagesOpts{ /* 与 API 默认一致 */ })
		if err != nil {
			b.Fatal(err)
		}
	}
}
```

先读 `internal/store` 中 `ListMessages` 真实签名与 opts，再写（禁止臆造字段名）。

- [ ] **步骤 2：跑通** `go test ./internal/store/ -bench=BenchmarkPerfListMessages -benchtime=20x -count=1`  
- [ ] **步骤 3：Commit** `test: 增加会话 ListMessages 性能探针`

---

### 任务 4：探针 C — blob Put/Get

**文件：**  
- 创建：`internal/blob/memory/perf_bench_test.go`  
- 创建：`internal/blob/file/perf_bench_test.go`

- [ ] **步骤 1：固定载荷**

`payload := bytes.Repeat([]byte("x"), 64*1024)`（64KiB）与可选 `256*1024` 第二组（可用 `b.Run`）。

```go
func BenchmarkPerfBlobPutGet64KiB(b *testing.B) {
	s, err := blob.Open(context.Background(), "memory", blob.Options{})
	// ...
	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("k-%d", i)
		if err := s.Put(ctx, key, payload, "application/octet-stream"); err != nil {
			b.Fatal(err)
		}
		if _, err := s.Get(ctx, key); err != nil {
			b.Fatal(err)
		}
	}
}
```

file 驱动：`blob.Open(..., "file", blob.Options{ /* Root: b.TempDir() 或 Options 字段以 registry 为准 */ })`——打开前读 `internal/blob/file` / `Open` 选项字段，勿猜错。

- [ ] **步骤 2：跑通 memory + file**  
- [ ] **步骤 3：Commit** `test: 增加 blob memory/file PutGet 性能探针`

---

### 任务 5：探针 D — outbound-deliveries 列表

**文件：** 创建 `internal/api/perf_outbox_bench_test.go`（包名与现有 `server_channel_outbox_test.go` 一致：`api_test` 或 `api`——**跟随邻文件**）

- [ ] **步骤 1：种子 K=200 条 outbox**

参考 `TestChannelOutboundDeliveriesListAndRetry`：`PutChannelOutboxIfAbsent`、注册 webhook channel、`AdminToken`、GET list。

`BenchmarkPerfOutboxList`：每次 `b.N` 发 GET `/v0/settings/channels/weixin/outbound-deliveries`，断言 200。

**禁止**设置真实可达的外网 `outbound_url` 并触发 Send；本探针只测 **List**（必要时单独子 benchmark「retry 入队」若可不发 HTTP——默认只做 List）。

- [ ] **步骤 2：跑通**  
- [ ] **步骤 3：更新 `scripts/perf-probes.ps1` 确认四条 bench 名匹配**  
- [ ] **步骤 4：Commit** `test: 增加渠道 outbox 列表性能探针`

---

### 任务 6：跑全量基线 → 填笔记 → 裁定是否优化

**文件：** 修改 `docs/superpowers/notes/2026-09-15-clean-perf-hot.md`

- [ ] **步骤 1：跑脚本**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
.\scripts\perf-probes.ps1
```

- [ ] **步骤 2：填笔记**

写入：`go version`、OS、四探针各次 `ns/op`（或脚本摘要）、中位数、驱动说明（memory/sqlite/file）。

- [ ] **步骤 3：裁定**

对照规格：是否存在**清晰、可复现**的优化点且预期 ≥20%？

- **否（默认预期）：** 结论写「四条已测、保持现状」；**跳过任务 7**（在笔记与 progress 标明 N/A）。  
- **是：** 在笔记写明目标文件、假设、预期指标；进入任务 7。

- [ ] **步骤 4：Commit** `docs: CLEAN-PERF-HOT 基线测量与优化裁定`

---

### 任务 7（条件）：有证据优化

**仅当任务 6 裁定「是」。** 否则整任务勾选为跳过并在笔记保留 N/A。

- [ ] **步骤 1：** 最小改动 + 相关单元测试  
- [ ] **步骤 2：** 重跑对应 `-bench -count=3`，笔记追加「优化后」中位数与改善百分比  
- [ ] **步骤 3：** 若改善 &lt;20%：**回滚优化提交**，改回「保持现状」  
- [ ] **步骤 4：** Commit（若保留）`perf: …`（中文说明具体路径）

---

### 任务 8：README 回填 + 账本收口

**文件：**  
- `README.md` / `README.zh-CN.md` 性能说明节  
- `docs/superpowers/notes/2026-09-13-spec-ledger.md`  
- `docs/superpowers/specs/2026-09-15-oss-quality-cleanup-design.md` / PERF 详设状态  
- 可选：AUDIT §7 加一句「PERF-HOT 已测」

- [ ] **步骤 1：README**

若有数字：表格列出探针名、中位数、环境一句、链到 `docs/developers/performance.md`。  
若无优化：明确写「已在本地跑完四探针；当前实现保持现状；复现见 performance.md」——**仍不要留「将在后续 PERF 回填」空头支票**。

- [ ] **步骤 2：账本** — PERF-HOT **已交付**；CLEAN 史诗标完成或「质量收口完毕」  
- [ ] **步骤 3：回归**

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
go test ./internal/api/ ./internal/store/ ./internal/blob/... -count=1
Push-Location web\chat; npm run lint; npm test; npx tsc --noEmit; Pop-Location
```

（bench 不进默认 CI。）

- [ ] **步骤 4：Commit** `docs: CLEAN-PERF-HOT 交付与 README/账本收口`  
- [ ] **步骤 5：停住** — finishing 菜单；提示双推后看 CI

---

## 自检（对照规格）

| 规格项 | 任务 |
|--------|------|
| 探针 A–D 全做 | 2–5 |
| 可复现脚本/文档 | 1、5、8 |
| 私有基线笔记 | 1、6 |
| ≥20% 才优化 | 6–7 |
| README 有证据或诚实「保持现状」 | 8 |
| 禁竞品/禁真外网 | 头部、5 |
| 账本收口 | 8 |

无占位步骤。
