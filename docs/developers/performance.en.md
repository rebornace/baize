[中文](./performance.md) | **English**

# Performance probes (local)

For contributors reproducing hot-path timings on their machine. Use your local Go (`go version`); results vary with hardware and load.

```powershell
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"
.\scripts\perf-probes.ps1
```

Or individually:

```powershell
go test ./internal/api/ -bench=BenchmarkPerfStreamReplay -benchtime=50x -count=3
go test ./internal/store/ -bench=BenchmarkPerfListMessages -benchtime=50x -count=3
go test ./internal/blob/memory/ ./internal/blob/file/ -bench=BenchmarkPerfBlobPutGet -benchtime=50x -count=3
go test ./internal/api/ -bench=BenchmarkPerfOutboxList -benchtime=50x -count=3
```

Prefer at least 3 runs on the same machine/command and take the median before deciding on code changes.
