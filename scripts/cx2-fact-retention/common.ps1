# Common helpers for the CX2 fact-retention evaluation harness.
# Dot-source shared helpers from tool-routing-eval.
. (Join-Path $PSScriptRoot '..\tool-routing-eval\common.ps1')

function Get-RuntimeEffective {
  param([string]$BaseUrl, [string]$ApiKey)
  $f = [System.IO.Path]::GetTempFileName()
  try {
    curl.exe -s "$BaseUrl/v0/settings/runtime" -H "Authorization: Bearer $ApiKey" -o $f | Out-Null
    $j = Get-Content $f -Raw -Encoding UTF8 | ConvertFrom-Json
    return $j.effective
  } finally { Remove-Item $f -ErrorAction SilentlyContinue }
}

function Assert-OffloadKnob {
  param([string]$BaseUrl, [string]$ApiKey, [bool]$Expected)
  $eff = Get-RuntimeEffective -BaseUrl $BaseUrl -ApiKey $ApiKey
  $got = [bool]$eff.context_offload_enabled
  if ($got -ne $Expected) {
    throw "context_offload_enabled want=$Expected got=$got"
  }
}

function Wait-BaizeRunTerminal {
  param(
    [string]$BaseUrl, [string]$ApiKey, [string]$RunID,
    [int]$TimeoutSec = 120
  )
  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  $status = 'running'
  while ((Get-Date) -lt $deadline) {
    Start-Sleep -Milliseconds 1500
    $g = Invoke-RestMethod -Uri "$BaseUrl/v0/runs/$RunID" `
      -Headers @{ Authorization = "Bearer $ApiKey" } -TimeoutSec 10
    $status = $g.status
    if ($status -in @('succeeded', 'failed', 'cancelled', 'waiting_human', 'rejected')) { break }
  }
  if ($status -in @('waiting_human', 'rejected', 'running')) {
    curl.exe -s -X POST "$BaseUrl/v0/runs/$RunID/cancel" `
      -H "Authorization: Bearer $ApiKey" | Out-Null
    if ($status -eq 'running') { $status = 'timeout' }
  }
  return $status
}

function Get-RunEvents {
  param([string]$BaseUrl, [string]$ApiKey, [string]$RunID, [string]$OutFile)
  curl.exe -s "$BaseUrl/v0/runs/$RunID/events" -H "Authorization: Bearer $ApiKey" -o $OutFile | Out-Null
  # PS5: assign ConvertFrom-Json first, then wrap — inline @() collapses arrays.
  $parsed = Get-Content $OutFile -Raw -Encoding UTF8 | ConvertFrom-Json
  return @($parsed)
}

# Final assistant text from llm.message events (internal/run EventLLMMessage).
# Engine writes Data["content"]; also accept text / string data for harness drift.
function Get-AssistantText {
  param($Events)
  $texts = @()
  foreach ($ev in @($Events)) {
    if ($ev.type -ne 'llm.message') { continue }
    $d = $ev.data
    $candidate = $null
    if ($null -eq $d) { continue }
    if ($d -is [string]) {
      $candidate = $d
    } else {
      if ($null -ne $d.content -and "$($d.content)".Length -gt 0) { $candidate = [string]$d.content }
      elseif ($null -ne $d.text -and "$($d.text)".Length -gt 0) { $candidate = [string]$d.text }
      elseif ($null -ne $d.data -and "$($d.data)".Length -gt 0) { $candidate = [string]$d.data }
    }
    if (-not [string]::IsNullOrWhiteSpace($candidate)) {
      $texts += $candidate
    }
  }
  if ($texts.Count -eq 0) { return '' }
  return [string]$texts[-1]
}

function Test-ExpectHit {
  param([string]$Answer, [string]$Expect)
  if ([string]::IsNullOrWhiteSpace($Answer) -or [string]::IsNullOrWhiteSpace($Expect)) { return $false }
  return $Answer.IndexOf($Expect, [StringComparison]::OrdinalIgnoreCase) -ge 0
}

function New-BulkyPaste {
  param(
    [string]$SecretTail,
    # Optional extra "key=value" lines buried after the pad (all facts for bulky cases).
    [string[]]$ExtraSecretLines = @()
  )
  # >= 64KiB runes; secrets at end so offload preview (500) cannot contain them
  $pad = -join (1..70000 | ForEach-Object { 'x' })
  $tail = "`nSECRET_ID=" + $SecretTail
  foreach ($line in @($ExtraSecretLines)) {
    if (-not [string]::IsNullOrWhiteSpace($line)) { $tail += "`n" + $line }
  }
  return ($pad + $tail)
}

function New-CompactPad {
  param([int]$Round = 1)
  # ~4k runes of filler so short scenarios eventually exceed compact budget
  # and leave foldable history beyond compact_keep_recent.
  $chunk = -join (1..4000 | ForEach-Object { 'p' })
  return ("【填充 $Round】请只回复「收到」。`n" + $chunk)
}
