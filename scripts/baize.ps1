# Shared Windows launcher: go run ./cmd/baize <args>
# Usage: .\baize.cmd start | demo | serve -config ...
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

if ($args.Count -eq 0) {
    Write-Host "usage: baize <start|demo|serve> ..."
    Write-Host "  start  production (needs BAIZE_API_KEY — copy .env.example to .env)"
    Write-Host "  demo   trial mock LLM + demo HTTP (no API key)"
    exit 2
}

$sub = $args[0]
if ($sub -eq "start" -and -not $env:BAIZE_API_KEY) {
    $envFile = Join-Path $Root ".env"
    if (Test-Path $envFile) {
        Get-Content $envFile | ForEach-Object {
            if ($_ -match '^\s*#' -or $_ -notmatch '^\s*([^#=]+)=(.*)$') { return }
            $name = $matches[1].Trim()
            $value = $matches[2].Trim().Trim('"').Trim("'")
            if ($name -and -not [string]::IsNullOrWhiteSpace($value)) {
                Set-Item -Path "Env:$name" -Value $value
            }
        }
    }
}

Write-Host "go run ./cmd/baize $args  (cwd=$Root)"
& go run ./cmd/baize @args
exit $LASTEXITCODE
