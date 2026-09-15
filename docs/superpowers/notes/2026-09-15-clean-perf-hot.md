# CLEAN-PERF-HOT 基线笔记

> 日期：2026-09-15  
> 计划：[`../plans/2026-09-15-clean-perf-hot.md`](../plans/2026-09-15-clean-perf-hot.md)  
> 规格：[`../specs/2026-09-15-clean-perf-hot-design.md`](../specs/2026-09-15-clean-perf-hot-design.md)  
> 复现：[`../../developers/performance.md`](../../developers/performance.md)

## 环境

| 项 | 值 |
|----|-----|
| `go version` | go1.25.0 windows/amd64 |
| OS | Windows 10.0.19045 (windows/amd64) |
| 机器摘要 | Intel(R) Core(TM) i9-10900KF CPU @ 3.70GHz；`-20` GOMAXPROCS |
| 命令 | `.\scripts\perf-probes.ps1`（PATH 含 `~\.local\go1.25.0\bin`；`-benchtime=50x -count=3`） |
| 原始输出 | `%TEMP%\baize-perf-probes.txt`（本机 2026-09-15 跑通） |

## 探针结果

阈值：同机同命令 ≥3 次取中位数；相对改善 ≥20% 才开优化。

### A — Run SSE stream（`BenchmarkPerfStreamReplay`）

| 次 | ns/op | B/op | allocs/op |
|----|------:|-----:|----------:|
| 1 | 114096 | 85533 | 744 |
| 2 | 115324 | 85520 | 744 |
| 3 | 112508 | 85527 | 744 |
| 中位数 | 114096 | 85527 | 744 |

驱动 / 测量段说明：memory store；**仅 terminal-replay**（run `StatusSucceeded`，回放 N=100 `llm.message` + SSE 写出后退出；无 live Hub / LLM）。

### B — ListMessages（`BenchmarkPerfListMessages`）

| 次 | ns/op | B/op | allocs/op |
|----|------:|-----:|----------:|
| 1 | 1267510 | 455120 | 11030 |
| 2 | 1257388 | 455018 | 11030 |
| 3 | 1258472 | 455017 | 11030 |
| 中位数 | 1258472 | 455018 | 11030 |

驱动说明：**sqlite**（`store.Open("sqlite", …)` + `conversation.OpenSQL`）；测的是生产路径 `conversation.Messages.List(convID)`（无 limit；N=500），与 `handleListMessages` 一致——**不是**独立 `ListMessages` API。

### C — blob Put/Get（`BenchmarkPerfBlobPutGet*`）

| 驱动 | 载荷 | 次1 ns/op | 次2 | 次3 | 中位数 |
|------|------|----------:|----:|----:|-------:|
| memory | 64KiB | 40676 | 26560 | 22886 | 26560 |
| memory | 256KiB | 103428 | 57412 | 83734 | 83734 |
| file | 64KiB | 411754 | 391620 | 397644 | 397644 |
| file | 256KiB | 466522 | 466730 | 457510 | 466522 |

### D — outbound-deliveries 列表（`BenchmarkPerfOutboxList`）

| 次 | ns/op | B/op | allocs/op |
|----|------:|-----:|----------:|
| 1 | 98792 | 225595 | 100 |
| 2 | 86084 | 226202 | 100 |
| 3 | 106662 | 225599 | 100 |
| 中位数 | 98792 | 225599 | 100 |

说明：仅 List（K=200 种子）；禁止真外网出站。

## 结论

四条探针已在本机用 `perf-probes.ps1` 跑通（各 `-count=3` 取中位数）。相对量级：stream replay ~0.11ms、outbox list ~0.10ms、blob memory Put/Get 64KiB ~0.03ms、file 64KiB ~0.40ms、sqlite `Messages.List` 500 条 ~1.26ms。未见明显 bug 级热点或可一眼指向具体文件且预期 ≥20% 的优化机会。

## 优化决定

| 项 | 值 |
|----|-----|
| 是否优化 | **否 → 保持现状** |
| 依据 | 默认预期；无清晰可复现 ≥20% 机会与具体文件假设。数值属本地探针基线，非 SLA。 |
| 目标路径 | N/A |
| 任务 7 | **N/A（跳过）** |
