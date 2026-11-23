# Shared cosign public-key fingerprint helper.
# Dot-source from sign-release.ps1 / rotate-release-key.ps1.

function Get-CosignPublicKeyFingerprint {
    param([Parameter(Mandatory = $true)][string]$Path)
    $pem = Get-Content -LiteralPath $Path -Raw
    $body = ($pem -replace '-----[^-]+-----', '' -replace '\s', '')
    $der = [Convert]::FromBase64String($body)
    $sha = [System.Security.Cryptography.SHA256]::Create()
    try {
        $hash = $sha.ComputeHash($der)
    } finally {
        $sha.Dispose()
    }
    return (($hash | ForEach-Object { $_.ToString('x2') }) -join '')
}
