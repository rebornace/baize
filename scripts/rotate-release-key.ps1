# Generate or inspect a local cosign release key without exposing it.
# Usage:
#   $env:COSIGN_PASSWORD = '...'
#   .\scripts\rotate-release-key.ps1 -OutDir D:\offline\baize-release-key-2026-11
#   .\scripts\rotate-release-key.ps1 -OutDir D:\offline\... -CheckOnly
#
# The private key never enters the repo. Hand off the directory over an
# encrypted channel, or keep it on offline media. See docs/developers/release-signing.md.
param(
    [Parameter(Mandatory = $true)][string]$OutDir,
    [string]$ExpectedFingerprint = "",
    [switch]$CheckOnly
)

$ErrorActionPreference = "Stop"
. (Join-Path $PSScriptRoot 'cosign-key.ps1')

$cosign = (Get-Command cosign -ErrorAction SilentlyContinue).Source
if (-not $cosign) {
    throw "cosign not found on PATH. Install it from https://docs.sigstore.dev/cosign/installation/"
}

$key = Join-Path $OutDir "cosign.key"
$pub = Join-Path $OutDir "cosign.pub"
$handoff = Join-Path $OutDir "KEY-HANDOFF.txt"

function Show-Fingerprint([string]$path) {
    $fp = Get-CosignPublicKeyFingerprint -Path $path
    Write-Host "cosign public key: $path"
    Write-Host "SHA256 fingerprint: $fp"
    if ($ExpectedFingerprint -and $ExpectedFingerprint.Trim().ToLowerInvariant() -ne $fp) {
        throw "public key fingerprint mismatch: expected $ExpectedFingerprint, got $fp"
    }
    return $fp
}

if ($CheckOnly) {
    if (-not (Test-Path $key) -or -not (Test-Path $pub)) {
        throw "CheckOnly requires $key and $pub"
    }
    $fp = Show-Fingerprint $pub
    Write-Host "CheckOnly OK; no files written."
    exit 0
}

if (Test-Path $key) {
    throw "refusing to overwrite existing private key at $key; choose a new -OutDir or move it first"
}
if (-not $env:COSIGN_PASSWORD) {
    throw "COSIGN_PASSWORD is required to encrypt the new private key"
}
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

Push-Location $OutDir
try {
    & $cosign generate-key-pair | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "cosign generate-key-pair failed"
    }
} finally {
    Pop-Location
}

$fp = Show-Fingerprint $pub
$stamp = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
@(
    "public_key=cosign.pub"
    "sha256=$fp"
    "created_at=$stamp"
    ""
    "Next:"
    "1. Store cosign.key + its password separately (offline media / vault)."
    "2. Sign with: .\scripts\sign-release.ps1 -Key $key -ArtifactsDir <dir> -OciArchive <oci> -ExpectedFingerprint $fp"
    "3. Publish cosign.pub and cosign-key-fingerprint.txt with the next Release."
    "4. Do not commit, chat, or email the private key."
) | Set-Content -LiteralPath $handoff -Encoding ascii

Write-Host "Wrote $handoff"
Write-Host "Private key: $key (do not share in chat/email)"
Write-Host "Next: sign with -ExpectedFingerprint $fp"
