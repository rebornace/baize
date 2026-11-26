[中文](./performance.md) | **English**

# Performance probes (local)

For contributors reproducing hot-path timings on their machine. Use your local Go (`go version`); results vary with hardware and load.

```powershell
.\scripts\perf-probes.ps1
```

Or individually:

```powershell
go test ./internal/api/ -bench=BenchmarkPerfStreamReplay -benchtime=50x -count=3
go test ./internal/store/ -bench=BenchmarkPerfListMessages -benchtime=50x -count=3
go test ./internal/blob/memory/ ./internal/blob/file/ -bench=BenchmarkPerfBlobPutGet -benchtime=50x -count=3
go test ./internal/api/ -bench=BenchmarkPerfOutboxList -benchtime=50x -count=3
go test ./internal/store/ -bench=BenchmarkSumRunCostsPerDayGrouped -benchtime=20x -count=3
```

Cost trends/anomalies use `SumRunCostsPerDayGrouped` (grouped by `rc.day`). On Postgres:

```sql
EXPLAIN (ANALYZE, BUFFERS)
SELECT rc.day, COALESCE(r.conversation_id, ''), COALESCE(SUM(rc.cost_usd), 0)
FROM run_costs rc JOIN runs r ON r.id = rc.run_id
WHERE rc.day >= '2026-01-01' AND rc.day <= '2026-01-07'
GROUP BY 1, 2;
```

Check that indexes on `run_costs(day)` and `runs(id)` are used; if the plan shows a sequential scan as data grows, revisit the indexes.

Prefer at least 3 runs on the same machine/command and take the median before deciding on code changes.
