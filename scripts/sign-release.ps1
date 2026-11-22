# Sign release artifacts with a local cosign key.
# Usage:
#   .\scripts\sign-release.ps1 -Key R:\keys\cosign.key `
#     -ArtifactsDir dist\release\v0.34.0 `
#     -OciArchive R:\out\baize-oci.tar
#
# The private key never enters the repo. Publish cosign.pub and the
# *.sigstore.json bundles next to the artifacts.
param(
    [Parameter(Mandatory = $true)][string]$Key,
    [string]$PubKey = "",
    [string]$ArtifactsDir = "",
    [string]$OciArchive = "",
    [string]$OutDir = ""
)

$ErrorActionPreference = "Stop"

$cosign = (Get-Command cosign -ErrorAction SilentlyContinue).Source
if (-not $cosign) {
    throw "cosign not found on PATH. Install it from https://docs.sigstore.dev/cosign/installation/"
}
if (-not (Test-Path $Key)) {
    throw "cosign private key not found: $Key"
}
if (-not $PubKey) {
    $PubKey = [System.IO.Path]::ChangeExtension($Key, ".pub")
}

function Sign-Artifact([string]$file, [string]$outDir) {
    if (-not (Test-Path $file)) {
        throw "artifact not found: $file"
    }
    if (-not $outDir) {
        $outDir = Split-Path -Parent $file
    }
    New-Item -ItemType Directory -Force -Path $outDir | Out-Null
    $bundle = Join-Path $outDir ((Split-Path -Leaf $file) + ".sigstore.json")
    Write-Host "Signing $file"
    & $cosign sign-blob --yes --key $Key --bundle $bundle $file | Out-Null
    if ($LASTEXITCODE -ne 0) {
        throw "cosign sign-blob failed: $file"
    }
    Write-Host "Wrote $bundle"
    return [pscustomobject]@{ File = $file; Bundle = $bundle }
}

$signed = @()
if ($ArtifactsDir) {
    $sums = Join-Path $ArtifactsDir "SHA256SUMS.txt"
    $signed += Sign-Artifact $sums $OutDir
}
if ($OciArchive) {
    $signed += Sign-Artifact $OciArchive $OutDir
}
if ($signed.Count -eq 0) {
    throw "nothing to sign: pass -ArtifactsDir and/or -OciArchive"
}

if (Test-Path $PubKey) {
    foreach ($item in $signed) {
        Write-Host "Verifying $($item.File)"
        & $cosign verify-blob --key $PubKey --bundle $item.Bundle --offline $item.File
        if ($LASTEXITCODE -ne 0) {
            throw "cosign verify-blob failed: $($item.File)"
        }
    }
} else {
    Write-Warning "public key not found at $PubKey; skipping verification"
}
