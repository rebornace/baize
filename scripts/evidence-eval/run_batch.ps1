# Live evidence-to-action batch: POST /v0/runs, wait, score trajectories.
# Requires a running baize with mock-ticket tools (list/get/create/update).
# Runs that enter waiting_human are scored as-is (HITL attempt counts).
param(
  [string]$CasesFile = (Join-Path $PSScriptRoot 'cases.json'),
  [string]$OutDir    = (Join-Path $PSScriptRoot 'out\runs'),
  [string]$AgentId   = 'ticket-agent',
  [string]$BaseUrl   = '',
  [string]$ApiKey    = '',
  [int]$TimeoutSec   = 120
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$cfg = Resolve-BaizeConfig -BaseUrl $BaseUrl -ApiKey $ApiKey
$base = $cfg.BaseUrl
$key  = $cfg.ApiKey

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$parsedCases = Get-Content $CasesFile -Raw -Encoding UTF8 | ConvertFrom-Json
$cases = @($parsedCases)
$results = @()
$passCount = 0

foreach ($case in $cases) {
  $id = [string]$case.id
  Write-Host ("-- {0} ({1})" -f $id, $case.protocol)

  $create = Send-BaizeRun -BaseUrl $base -ApiKey $key -AgentID $AgentId `
    -Text ([string]$case.input) -ConversationID ''
  $rid = [string]$create.run_id
  if (-not $rid) {
    Write-Host "FAIL $id: no run_id from create"
    $results += [pscustomobject]@{ id = $id; pass = $false; codes = @('NO_RUN'); detail = 'create failed'; calls = ''; run_status = 'error' }
    continue
  }

  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  $status = 'running'
  while ((Get-Date) -lt $deadline) {
    Start-Sleep -Milliseconds 1500
    $f = [System.IO.Path]::GetTempFileName()
    try {
      curl.exe -s "$base/v0/runs/$rid" -H "Authorization: Bearer $key" -o $f | Out-Null
      $rec = Get-Content $f -Raw -Encoding UTF8 | ConvertFrom-Json
      $status = [string]$rec.status
      # Terminal for scoring: finished, or parked on HITL (evidence attempt visible).
      if ($status -in @('succeeded', 'failed', 'cancelled', 'waiting_human')) { break }
    } finally { Remove-Item $f -ErrorAction SilentlyContinue }
  }
  if ($status -eq 'running') { $status = 'timeout' }

  $evFile = Join-Path $OutDir ($id + '.events.json')
  curl.exe -s "$base/v0/runs/$rid/events" -H "Authorization: Bearer $key" -o $evFile
  $parsedEvs = Get-Content $evFile -Raw -Encoding UTF8 | ConvertFrom-Json
  $evs = @($parsedEvs)
  $tl = Get-ToolTimeline -Events $evs
  $scored = Score-EvidenceCase -Case $case -Timeline $tl -RunStatus $status
  if ($scored.pass) { $passCount++ }

  $row = [pscustomobject]@{
    id         = $scored.id
    protocol   = $scored.protocol
    pass       = $scored.pass
    codes      = ($scored.codes -join ',')
    detail     = $scored.detail
    calls      = $scored.calls
    run_status = $status
    run_id     = $rid
  }
  $results += $row
  Write-Host ("[{0}] pass={1} status={2} codes=[{3}] calls={4}" -f `
    $id, $scored.pass, $status, ($scored.codes -join ','), $scored.calls)
}

$summaryPath = Join-Path $OutDir 'summary.json'
$results | ConvertTo-Json -Depth 4 | Out-File $summaryPath -Encoding utf8
Write-Host ("evidence-eval: {0}/{1} passed → {2}" -f $passCount, $results.Count, $summaryPath)
$results | Format-Table -AutoSize | Out-String -Width 220
if ($passCount -lt $results.Count) { exit 1 }
