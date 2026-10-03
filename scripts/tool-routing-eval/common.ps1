# Common helpers for the tool-routing evaluation harness.
# Dot-source from the sibling scripts: . (Join-Path $PSScriptRoot 'common.ps1')

# Load KEY=VALUE lines from an env file into the current process (without
# overriding variables that are already set in the environment).
function Import-BaizeEnv {
  param([string]$Path)
  if (-not (Test-Path $Path)) { return }
  Get-Content $Path | ForEach-Object {
    if ($_ -match '^\s*([^#=]+)=(.*)$') {
      $name = $Matches[1].Trim()
      if (-not [Environment]::GetEnvironmentVariable($name)) {
        [Environment]::SetEnvironmentVariable($name, $Matches[2].Trim(), 'Process')
      }
    }
  }
}

# Resolve base URL + API key from explicit params, then environment, then an
# optional .env file. Returns an ordered hashtable.
function Resolve-BaizeConfig {
  param(
    [string]$BaseUrl,
    [string]$ApiKey,
    [string]$EnvFile = (Join-Path $PSScriptRoot '..\..\.env')
  )
  if (-not $ApiKey -and -not $env:BAIZE_API_KEY) { Import-BaizeEnv $EnvFile }
  if (-not $BaseUrl) { $BaseUrl = if ($env:BAIZE_BASE_URL) { $env:BAIZE_BASE_URL } else { 'http://127.0.0.1:8080' } }
  if (-not $ApiKey) { $ApiKey = $env:BAIZE_API_KEY }
  if (-not $ApiKey) { throw 'No API key: pass -ApiKey, set BAIZE_API_KEY, or provide an .env file.' }
  return [ordered]@{ BaseUrl = $BaseUrl.TrimEnd('/'); ApiKey = $ApiKey }
}

# Hot-patch runtime knobs. The JSON body is written as UTF-8 (no BOM) and sent
# with curl so non-ASCII content survives; capturing curl stdout in a localized
# PowerShell would transcode through the OEM codepage.
function Set-RuntimeKnobs {
  param(
    [string]$BaseUrl,
    [string]$ApiKey,
    [string]$Json
  )
  $tmp = [System.IO.Path]::GetTempFileName()
  try {
    [System.IO.File]::WriteAllText($tmp, $Json, (New-Object System.Text.UTF8Encoding $false))
    curl.exe -s -X PATCH "$BaseUrl/v0/settings/runtime" `
      -H "Authorization: Bearer $ApiKey" -H 'Content-Type: application/json' `
      --data-binary "@$tmp" | Out-Null
  } finally {
    Remove-Item $tmp -ErrorAction SilentlyContinue
  }
}

# Send one POST /v0/runs (UTF-8 body via curl). Returns the parsed run record.
function Send-BaizeRun {
  param(
    [string]$BaseUrl, [string]$ApiKey, [string]$AgentID,
    [string]$Text, [string]$ConversationID
  )
  $bodyFile = [System.IO.Path]::GetTempFileName()
  $respFile = [System.IO.Path]::GetTempFileName()
  try {
    $payload = @{ agent_id = $AgentID; input = $Text; conversation_id = $ConversationID } |
      ConvertTo-Json -Compress
    [System.IO.File]::WriteAllText($bodyFile, $payload, (New-Object System.Text.UTF8Encoding $false))
    curl.exe -s -X POST "$BaseUrl/v0/runs" `
      -H "Authorization: Bearer $ApiKey" -H 'Content-Type: application/json' `
      --data-binary "@$bodyFile" -o $respFile | Out-Null
    return (Get-Content $respFile -Raw -Encoding UTF8 | ConvertFrom-Json)
  } finally {
    Remove-Item $bodyFile, $respFile -ErrorAction SilentlyContinue
  }
}

# Poll a run until terminal; returns the final status.
function Wait-BaizeRun {
  param(
    [string]$BaseUrl, [string]$ApiKey, [string]$RunID,
    [int]$PollIntervalSec = 2, [int]$PollTimeoutSec = 240
  )
  $deadline = (Get-Date).AddSeconds($PollTimeoutSec)
  while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds $PollIntervalSec
    $f = [System.IO.Path]::GetTempFileName()
    try {
      curl.exe -s "$BaseUrl/v0/runs/$RunID" -H "Authorization: Bearer $ApiKey" -o $f | Out-Null
      $rec = Get-Content $f -Raw -Encoding UTF8 | ConvertFrom-Json
      if ($rec.status -in @('succeeded', 'failed', 'cancelled')) { return $rec.status }
    } finally { Remove-Item $f -ErrorAction SilentlyContinue }
  }
  return 'timeout'
}

# Return the data objects of a run's tokens.estimate events.
function Get-RunTokenEstimates {
  param([string]$BaseUrl, [string]$ApiKey, [string]$RunID)
  $f = [System.IO.Path]::GetTempFileName()
  try {
    curl.exe -s "$BaseUrl/v0/runs/$RunID/events" -H "Authorization: Bearer $ApiKey" -o $f | Out-Null
    $evs = Get-Content $f -Raw -Encoding UTF8 | ConvertFrom-Json
    @($evs) | Where-Object { $_.type -eq 'tokens.estimate' } | ForEach-Object { $_.data }
  } finally { Remove-Item $f -ErrorAction SilentlyContinue }
}
