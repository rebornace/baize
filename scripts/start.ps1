# Baize local one-shot launcher (Windows).
# Usage (from repo root):  .\start.cmd
# Or:                      .\scripts\start.ps1

$ErrorActionPreference = "Stop"

$Root = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $Root

$sdkGo = Join-Path $env:USERPROFILE "sdk\go\bin"
if (Test-Path (Join-Path $sdkGo "go.exe")) {
    $env:Path = "$sdkGo;" + $env:Path
}

if (-not $env:GOPROXY) {
    $env:GOPROXY = "https://goproxy.cn,direct"
}
if (-not $env:GOSUMDB) {
    $env:GOSUMDB = "sum.golang.google.cn"
}

$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    Write-Error "go not found. Install Go 1.22+ or put it on PATH (e.g. %USERPROFILE%\sdk\go\bin)."
}

Write-Host "baize start  (cwd=$Root  go=$($go.Source))"
& go run ./cmd/baize start @args
exit $LASTEXITCODE