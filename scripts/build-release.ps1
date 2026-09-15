# Cross-compile baize + weixin-adapter for common OS/arch pairs.
# Usage (repo root): .\scripts\build-release.ps1 -Version v0.1.0
param(
    [Parameter(Mandatory = $true)]
    [string]$Version,
    [string]$OutDir = ""
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
if (-not $OutDir) {
    $OutDir = Join-Path $Root "dist\release\$Version"
}

$targets = @(
    @{ GOOS = "linux"; GOARCH = "amd64"; Ext = ""; Archive = "tar.gz" },
    @{ GOOS = "linux"; GOARCH = "arm64"; Ext = ""; Archive = "tar.gz" },
    @{ GOOS = "darwin"; GOARCH = "amd64"; Ext = ""; Archive = "tar.gz" },
    @{ GOOS = "darwin"; GOARCH = "arm64"; Ext = ""; Archive = "tar.gz" },
    @{ GOOS = "windows"; GOARCH = "amd64"; Ext = ".exe"; Archive = "zip" },
    @{ GOOS = "windows"; GOARCH = "arm64"; Ext = ".exe"; Archive = "zip" }
)

if (Test-Path $OutDir) {
    Remove-Item -Recurse -Force $OutDir
}
New-Item -ItemType Directory -Path $OutDir -Force | Out-Null

$ldflags = "-s -w"
$env:CGO_ENABLED = "0"

Push-Location $Root
try {
    foreach ($t in $targets) {
        $name = "baize_${Version}_$($t.GOOS)_$($t.GOARCH)"
        $stage = Join-Path $OutDir $name
        New-Item -ItemType Directory -Path $stage -Force | Out-Null
        New-Item -ItemType Directory -Path (Join-Path $stage "configs") -Force | Out-Null

        $env:GOOS = $t.GOOS
        $env:GOARCH = $t.GOARCH

        $baizeOut = Join-Path $stage ("baize" + $t.Ext)
        $adapterOut = Join-Path $stage ("weixin-adapter" + $t.Ext)
        Write-Host "Building $name ..."
        go build -trimpath -ldflags $ldflags -o $baizeOut ./cmd/baize
        go build -trimpath -ldflags $ldflags -o $adapterOut ./cmd/weixin-adapter

        Copy-Item (Join-Path $Root "configs\minimal.yaml") (Join-Path $stage "configs\minimal.yaml")
        Copy-Item (Join-Path $Root ".env.example") (Join-Path $stage ".env.example")
        @"
Baize $Version ($($t.GOOS)/$($t.GOARCH))

1. Copy .env.example to .env and set BAIZE_API_KEY / BAIZE_SETTINGS_KEY.
2. Optionally edit configs/minimal.yaml (or create configs/minimal.local.yaml).
3. Run: ./baize start   (Windows: .\baize.exe start)
4. Open http://127.0.0.1:8080/ui

weixin-adapter is only needed for the Weixin channel.
Docs: https://github.com/rebornace/baize
"@ | Set-Content -Path (Join-Path $stage "README.txt") -Encoding utf8

        $archivePath = Join-Path $OutDir $name
        if ($t.Archive -eq "zip") {
            $zipPath = "$archivePath.zip"
            if (Test-Path $zipPath) { Remove-Item -Force $zipPath }
            # Prefer tar over Compress-Archive: AV/Go may briefly lock freshly built .exe on Windows.
            tar -a -cf $zipPath -C $OutDir $name
        } else {
            $tarPath = "$archivePath.tar.gz"
            if (Test-Path $tarPath) { Remove-Item -Force $tarPath }
            tar -czf $tarPath -C $OutDir $name
        }
        Remove-Item -Recurse -Force $stage
    }
}
finally {
    Remove-Item Env:GOOS -ErrorAction SilentlyContinue
    Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
    Pop-Location
}

# SHA256 sums
$sumFile = Join-Path $OutDir "SHA256SUMS.txt"
Get-ChildItem $OutDir -File | Where-Object { $_.Name -match '\.(zip|tar\.gz)$' } | ForEach-Object {
    $hash = (Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower()
    "$hash  $($_.Name)"
} | Set-Content -Path $sumFile -Encoding ascii

Write-Host "Done: $OutDir"
Get-ChildItem $OutDir | Format-Table Name, Length
