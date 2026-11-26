**中文** | [English](./performance.en.md)

# 性能探针（本地复现）

给贡献者在本机复现热点路径耗时用。使用当前机器的 Go（`go version`）；结果因硬件与负载而异。

```powershell
.\scripts\perf-probes.ps1
```

或单独：

```powershell
go test ./internal/api/ -bench=BenchmarkPerfStreamReplay -benchtime=50x -count=3
go test ./internal/store/ -bench=BenchmarkPerfListMessages -benchtime=50x -count=3
go test ./internal/blob/memory/ ./internal/blob/file/ -bench=BenchmarkPerfBlobPutGet -benchtime=50x -count=3
go test ./internal/api/ -bench=BenchmarkPerfOutboxList -benchtime=50x -count=3
go test ./internal/store/ -bench=BenchmarkSumRunCostsPerDayGrouped -benchtime=20x -count=3
```

费用趋势/异常使用 `SumRunCostsPerDayGrouped`（按 `rc.day` 分组）。Postgres 上可用：

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT rc.day, COALESCE(r.conversation_id, ''), COALESCE(SUM(rc.cost_usd), 0)
FROM run_costs rc JOIN runs r ON r.id = rc.run_id
WHERE rc.day >= '2026-01-01' AND rc.day <= '2026-01-07'
GROUP BY 1, 2;
```

关注 `run_costs(day)` 与 `runs(id)` 的索引使用；数据量增大后优先确认这里没有全表扫描。

同机同命令建议至少跑 3 次取中位数，再判断是否值得改代码。
