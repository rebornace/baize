#Requires -Version 5.1
<#
.SYNOPSIS
  POST a signed alert to Baize Webhook Inbox v1.

.DESCRIPTION
  Reads INBOX_SECRET and RUNTIME_URL from the environment, signs the JSON body
  with HMAC-SHA256 (v1=<hex>), and POSTs to /v0/inbox/{channel_id}.

  Signature: HMAC-SHA256(secret, "<unix_seconds>.<raw_body_bytes>")
  Header:    X-Baize-Inbox-Signature: v1=<lowercase_hex>

.EXAMPLE
  $env:RUNTIME_URL = "http://127.0.0.1:8080"
  $env:INBOX_SECRET = "<channel-secret-from-settings>"
  .\post.ps1
  .\post.ps1 -ChannelId alerts
#>
param(
    [string]$ChannelId = "alerts"
)

$secret = $env:INBOX_SECRET
$runtimeUrl = $env:RUNTIME_URL

if ([string]::IsNullOrWhiteSpace($secret)) {
    Write-Error "INBOX_SECRET is not set. Copy the channel secret from Settings -> Inbox (shown once on create or rotate)."
    exit 1
}
if ([string]::IsNullOrWhiteSpace($runtimeUrl)) {
    Write-Error "RUNTIME_URL is not set (e.g. http://127.0.0.1:8080)."
    exit 1
}

$bodyObj = [ordered]@{
    input           = "High CPU on web-01 (example alert from post.ps1)"
    idempotency_key = "alert-$(Get-Date -Format 'yyyyMMdd-HHmmss')"
    external_id     = "prometheus/alert/42"
    metadata        = @{
        source   = "prometheus"
        severity = "high"
    }
}
$body = ($bodyObj | ConvertTo-Json -Compress -Depth 4)

$ts = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds().ToString()
$signed = "$ts.$body"

$hmac = [System.Security.Cryptography.HMACSHA256]::new()
$hmac.Key = [System.Text.Encoding]::UTF8.GetBytes($secret)
$hash = $hmac.ComputeHash([System.Text.Encoding]::UTF8.GetBytes($signed))
$hex = -join ($hash | ForEach-Object { $_.ToString('x2') })
$sig = "v1=$hex"

$url = "$($runtimeUrl.TrimEnd('/'))/v0/inbox/$ChannelId"

try {
    $response = Invoke-WebRequest -Uri $url -Method POST `
        -ContentType 'application/json' `
        -Headers @{
            'X-Baize-Inbox-Timestamp' = $ts
            'X-Baize-Inbox-Signature' = $sig
        } `
        -Body ([System.Text.Encoding]::UTF8.GetBytes($body)) `
        -UseBasicParsing

    Write-Host "HTTP $($response.StatusCode)"
    Write-Host $response.Content
}
catch {
    if ($_.Exception.Response) {
        $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
        $errBody = $reader.ReadToEnd()
        Write-Error "HTTP $([int]$_.Exception.Response.StatusCode): $errBody"
    }
    else {
        Write-Error $_
    }
    exit 1
}
