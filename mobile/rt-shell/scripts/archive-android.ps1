param(
  [string]$Version = "1.0.0",
  [int]$VersionCode = 1,
  [string]$OutputDir = ""
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$KeystoreFile = Join-Path $Root "keystore.properties"
$Gradlew = Join-Path $Root "gradlew.bat"

if (-not (Test-Path -LiteralPath $KeystoreFile)) {
  throw "missing keystore.properties; copy keystore.properties.example and fill your local signing values"
}
if (-not (Test-Path -LiteralPath $Gradlew)) {
  throw "missing gradlew.bat; run the Gradle wrapper setup first"
}

$props = @{}
Get-Content -LiteralPath $KeystoreFile -Encoding UTF8 | ForEach-Object {
  if ($_ -match '^\s*([^#][^=]*)=(.*)$') {
    $props[$matches[1].Trim()] = $matches[2].Trim()
  }
}
foreach ($key in @("storeFile", "storePassword", "keyAlias", "keyPassword")) {
  if (-not $props.ContainsKey($key) -or [string]::IsNullOrWhiteSpace($props[$key])) {
    throw "keystore.properties missing $key"
  }
  if ($props[$key] -eq "change-me") {
    throw "keystore.properties still contains change-me; fill your local signing values"
  }
}
$StorePath = [IO.Path]::GetFullPath((Join-Path $Root $props["storeFile"]))
if (-not (Test-Path -LiteralPath $StorePath)) {
  throw "keystore file not found: $StorePath"
}

Push-Location $Root
try {
  & $Gradlew clean bundleRelease "-PversionName=$Version" "-PversionCode=$VersionCode"
  if ($LASTEXITCODE -ne 0) {
    throw "gradle bundleRelease failed with exit code $LASTEXITCODE"
  }
} finally {
  Pop-Location
}

$Aab = Join-Path $Root "app\build\outputs\bundle\release\app-release.aab"
if (-not (Test-Path -LiteralPath $Aab)) {
  throw "expected AAB not found: $Aab"
}
if ([string]::IsNullOrWhiteSpace($OutputDir)) {
  $OutputDir = Join-Path (Split-Path -Parent $Root) "..\dist\android"
}
$OutputDir = [IO.Path]::GetFullPath($OutputDir)
New-Item -ItemType Directory -Path $OutputDir -Force | Out-Null
$OutFile = Join-Path $OutputDir "BaizeMobileRT-$Version.aab"
Copy-Item -LiteralPath $Aab -Destination $OutFile -Force

Write-Host "AAB: $OutFile"
Write-Host "Next: upload to Play Console or validate locally with bundletool."
