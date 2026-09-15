# CLEAN-PERF-HOT 基线笔记

> 日期：2026-09-15  
> 计划：[`../plans/2026-09-15-clean-perf-hot.md`](../plans/2026-09-15-clean-perf-hot.md)  
> 规格：[`../specs/2026-09-15-clean-perf-hot-design.md`](../specs/2026-09-15-clean-perf-hot-design.md)  
> 复现：[`../../developers/performance.md`](../../developers/performance.md)

## 环境

| 项 | 值 |
|----|-----|
| `go version` | （待填） |
| OS | （待填） |
| 机器摘要 | （待填） |

## 探针结果

阈值：同机同命令 ≥3 次取中位数；相对改善 ≥20% 才开优化。

### A — Run SSE stream（`BenchmarkPerfStream*`）

| 次 | ns/op | B/op | allocs/op |
|----|------:|-----:|----------:|
| 1 | | | |
| 2 | | | |
| 3 | | | |
| 中位数 | | | |

驱动 / 测量段说明：（待填）

### B — ListMessages（`BenchmarkPerfListMessages`）

| 次 | ns/op | B/op | allocs/op |
|----|------:|-----:|----------:|
| 1 | | | |
| 2 | | | |
| 3 | | | |
| 中位数 | | | |

驱动说明（memory / sqlite）：（待填）

### C — blob Put/Get（`BenchmarkPerfBlob*`）

| 驱动 | 载荷 | 次1 ns/op | 次2 | 次3 | 中位数 |
|------|------|----------:|----:|----:|-------:|
| memory | 64KiB | | | | |
| file | 64KiB | | | | |

### D — outbound-deliveries 列表（`BenchmarkPerfOutboxList`）

| 次 | ns/op | B/op | allocs/op |
|----|------:|-----:|----------:|
| 1 | | | |
| 2 | | | |
| 3 | | | |
| 中位数 | | | |

说明：仅 List；禁止真外网出站。

## 结论

（待填：四条已测摘要）

## 优化决定

| 项 | 值 |
|----|-----|
| 是否优化 | （待裁定：是 / 否 → 保持现状） |
| 依据 | （中位数 / 绝对体感论证） |
| 目标路径 | N/A 或文件路径 |
| 任务 7 | N/A 或进行 |
