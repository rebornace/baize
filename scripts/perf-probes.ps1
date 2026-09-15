# CLEAN-PERF-HOT: run four local perf probes (bench x3) and print summary path.
# Usage: .\scripts\perf-probes.ps1
$ErrorActionPreference = "Stop"
$env:PATH = "$env:USERPROFILE\.local\go1.25.0\bin;$env:PATH"

$out = Join-Path $env:TEMP "baize-perf-probes.txt"
if (Test-Path $out) {
    Remove-Item -Force $out
}

# Bench regexes aligned to actual names from tasks 2–5:
# BenchmarkPerfStreamReplay, BenchmarkPerfListMessages,
# BenchmarkPerfBlobPutGet{64,256}KiB, BenchmarkPerfOutboxList
$cmds = @(
    @( "go", "test", "./internal/api/", "-bench=BenchmarkPerfStreamReplay", "-benchtime=50x", "-count=3" ),
    @( "go", "test", "./internal/store/", "-bench=BenchmarkPerfListMessages", "-benchtime=50x", "-count=3" ),
    @( "go", "test", "./internal/blob/memory/", "./internal/blob/file/", "-bench=BenchmarkPerfBlobPutGet", "-benchtime=50x", "-count=3" ),
    @( "go", "test", "./internal/api/", "-bench=BenchmarkPerfOutboxList", "-benchtime=50x", "-count=3" )
)

foreach ($cmd in $cmds) {
    $line = ($cmd -join " ")
    Add-Content -Path $out -Value ("=== " + $line + " ===")
    Write-Host $line
    & $cmd[0] $cmd[1..($cmd.Length - 1)] 2>&1 | Tee-Object -FilePath $out -Append
    if ($LASTEXITCODE -ne 0) {
        Write-Host "FAIL exit=$LASTEXITCODE — see $out"
        exit $LASTEXITCODE
    }
    Add-Content -Path $out -Value ""
}

Write-Host "perf probe output: $out"
