# Rebuild MOBILE-RT Android AAR (requires JDK 17+, Android NDK, gomobile).
# Usage (from repo root):
#   .\scripts\build-mobileagent-aar.ps1
#
# Env overrides:
#   JAVA_HOME, ANDROID_HOME, ANDROID_NDK_HOME, GOPROXY

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent (Split-Path -Parent $MyInvocation.MyCommand.Path)
Set-Location $Root

if (-not $env:JAVA_HOME) { $env:JAVA_HOME = "D:\tools\jdk-17.0.13+11" }
if (-not $env:ANDROID_HOME) { $env:ANDROID_HOME = "D:\android-sdk" }
if (-not $env:ANDROID_SDK_ROOT) { $env:ANDROID_SDK_ROOT = $env:ANDROID_HOME }
if (-not $env:ANDROID_NDK_HOME) {
    $ndk = Get-ChildItem (Join-Path $env:ANDROID_HOME "ndk") -Directory -ErrorAction SilentlyContinue |
        Sort-Object Name -Descending | Select-Object -First 1
    if (-not $ndk) { throw "ANDROID_NDK_HOME not set and no ndk under ANDROID_HOME" }
    $env:ANDROID_NDK_HOME = $ndk.FullName
}
if (-not $env:GOPROXY) { $env:GOPROXY = "https://goproxy.cn,direct" }
$env:Path = "$env:JAVA_HOME\bin;$env:USERPROFILE\go\bin;$env:Path"

$outDir = Join-Path $Root "mobile\rt-shell\app\libs"
New-Item -ItemType Directory -Force -Path $outDir | Out-Null
$aar = Join-Path $outDir "mobileagent.aar"

Write-Host "NDK=$env:ANDROID_NDK_HOME"
Write-Host "Binding ./mobileagent -> $aar"
& gomobile bind -target=android -androidapi 21 -o $aar ./mobileagent
if ($LASTEXITCODE -ne 0) { throw "gomobile bind failed" }
Get-Item $aar | Format-List FullName, Length, LastWriteTime
