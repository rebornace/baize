# 性能探针（本地）

环境：本机 Go（`go version`），非生产 SLA。

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
.\scripts\perf-probes.ps1
```

或单独：

```powershell
go test ./internal/api/ -bench=BenchmarkPerfStreamReplay -benchtime=50x -count=3
go test ./internal/store/ -bench=BenchmarkPerfListMessages -benchtime=50x -count=3
go test ./internal/blob/memory/ ./internal/blob/file/ -bench=BenchmarkPerfBlobPutGet -benchtime=50x -count=3
go test ./internal/api/ -bench=BenchmarkPerfOutboxList -benchtime=50x -count=3
```

阈值：优化须相对中位数改善 ≥20%，否则保持现状。禁止竞品对比数字。
