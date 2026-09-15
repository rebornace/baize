# 性能探针（本地复现）

给贡献者在本机复现热点路径耗时用。使用当前机器的 Go（`go version`）；结果因硬件与负载而异。

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

同机同命令建议至少跑 3 次取中位数，再判断是否值得改代码。
