param(
  [string]$RequestsFile = (Join-Path $PSScriptRoot 'requests.json'),
  [string]$OutDir       = (Join-Path $PSScriptRoot 'out\runs'),
  [string]$AgentId      = 'default-agent',
  [string]$BaseUrl      = '',
  [string]$ApiKey       = '',
  [int]$TimeoutSec      = 180
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$cfg = Resolve-BaizeConfig -BaseUrl $BaseUrl -ApiKey $ApiKey
$base = $cfg.BaseUrl; $key = $cfg.ApiKey
New-Item -ItemType Directory -Force -Path $OutDir | Out-Null

# Aggressive compact for this arm (projection stays untouched).
# keep_recent must be small: foldOlder no-ops when len(messages) <= keep.
Set-RuntimeKnobs -BaseUrl $base -ApiKey $key -Json (@{
  compaction_enabled  = $true
  compact_threshold   = 0.05
  compact_keep_recent = 2
  compact_reserve_tokens = 8000
} | ConvertTo-Json -Compress)

# PS5: assign ConvertFrom-Json first, then wrap — inline @() collapses arrays.
$parsedReqs = Get-Content $RequestsFile -Raw -Encoding UTF8 | ConvertFrom-Json
$reqs = @($parsedReqs)
$summary = @()

# Steer default-agent away from connector tools / HITL login gates.
$evalPrefix = "【评测】不要调用任何工具；只文字简短回复。`n"

foreach ($r in $reqs) {
  $conv = "cx2-" + $r.id + "-" + [guid]::NewGuid().ToString('N').Substring(0, 8)
  $caseDir = Join-Path $OutDir $r.id
  New-Item -ItemType Directory -Force -Path $caseDir | Out-Null
  $factMap = @{}
  foreach ($f in @($r.facts)) { $factMap[$f.key] = $f.expect }

  $invalid = $false
  $invalidReason = ''
  $compacted = $false
  $offloaded = $false
  $hits = 0
  $totalQ = @($r.questions).Count
  $lastStatus = ''

  function Test-HasEventType($Events, [string]$Type) {
    return (@($Events | Where-Object { $_.type -eq $Type }).Count -gt 0)
  }

  function Invoke-Turn([string]$text, [string]$tag) {
    $payload = $evalPrefix + $text
    $create = Send-BaizeRun -BaseUrl $base -ApiKey $key -AgentID $AgentId -Text $payload -ConversationID $conv
    $rid = $create.run_id
    $st = Wait-BaizeRunTerminal -BaseUrl $base -ApiKey $key -RunID $rid -TimeoutSec $TimeoutSec
    $evFile = Join-Path $caseDir ($tag + '.' + $rid + '.events.json')
    $evs = Get-RunEvents -BaseUrl $base -ApiKey $key -RunID $rid -OutFile $evFile
    return [pscustomobject]@{ RunId = $rid; Status = $st; Events = $evs }
  }

  foreach ($t in @($r.plant_turns)) {
    $tr = Invoke-Turn $t 'plant'
    $lastStatus = $tr.Status
    if (Test-HasEventType $tr.Events 'context.compacted') { $compacted = $true }
    if ($tr.Status -ne 'succeeded') { $invalid = $true; $invalidReason = 'plant_' + $tr.Status; break }
  }

  if (-not $invalid -and $r.bulky) {
    $secret = $factMap[$r.bulky_fact_key]
    $extra = @()
    foreach ($fk in $factMap.Keys) {
      $extra += ("SECRET_" + $fk + "=" + $factMap[$fk])
    }
    $tr = Invoke-Turn (New-BulkyPaste -SecretTail $secret -ExtraSecretLines $extra) 'bulky'
    $lastStatus = $tr.Status
    if (Test-HasEventType $tr.Events 'context.compacted') { $compacted = $true }
    if (Test-HasEventType $tr.Events 'context.offloaded') { $offloaded = $true }
    if ($tr.Status -ne 'succeeded') { $invalid = $true; $invalidReason = 'bulky_' + $tr.Status }
  }

  if (-not $invalid) {
    foreach ($t in @($r.filler_turns)) {
      $tr = Invoke-Turn $t 'filler'
      $lastStatus = $tr.Status
      if (Test-HasEventType $tr.Events 'context.compacted') { $compacted = $true }
    }
    if (-not $compacted) {
      $tr = Invoke-Turn $r.compact_nudge 'compact'
      $lastStatus = $tr.Status
      if (Test-HasEventType $tr.Events 'context.compacted') { $compacted = $true }
    }
    # Short cases lack bulky paste; pad until MaybeCompact fires (or give up).
    $padRound = 0
    while (-not $compacted -and $padRound -lt 10) {
      $padRound++
      $pad = New-CompactPad -Round $padRound
      $tr = Invoke-Turn $pad ('pad' + $padRound)
      $lastStatus = $tr.Status
      if (Test-HasEventType $tr.Events 'context.compacted') { $compacted = $true }
      if ($tr.Status -ne 'succeeded') { break }
    }
    if (-not $compacted) { $invalid = $true; $invalidReason = 'no_compact' }
  }

  $answers = @()
  if (-not $invalid) {
    $qi = 0
    foreach ($q in @($r.questions)) {
      $qi++
      $tr = Invoke-Turn $q.ask ('ask' + $qi)
      $lastStatus = $tr.Status
      $ans = Get-AssistantText -Events $tr.Events
      $expect = $factMap[$q.expect_key]
      $hit = Test-ExpectHit -Answer $ans -Expect $expect
      if ($hit) { $hits++ }
      $answers += [ordered]@{ ask = $q.ask; expect = $expect; answer = $ans; hit = $hit; status = $tr.Status }
    }
  }

  # live arm bulky must offload; baseline (off) must not require offload
  # caller may set CX2_EXPECT_OFFLOAD=1 (e.g. compare harness)
  if ($env:CX2_EXPECT_OFFLOAD -eq '1' -and $r.bulky -and -not $offloaded) {
    $invalid = $true
    $invalidReason = 'no_offload'
  }

  $row = [ordered]@{
    id              = $r.id
    conversation_id = $conv
    invalid         = $invalid
    invalid_reason  = $invalidReason
    compacted       = $compacted
    offloaded       = $offloaded
    hits            = $hits
    total_q         = $totalQ
    last_status     = $lastStatus
    answers         = $answers
  }
  $summary += $row
  [System.IO.File]::WriteAllText(
    (Join-Path $caseDir 'case.json'),
    ($row | ConvertTo-Json -Depth 8),
    (New-Object System.Text.UTF8Encoding $false))
}

$sumPath = Join-Path $OutDir 'summary.json'
[System.IO.File]::WriteAllText($sumPath, ($summary | ConvertTo-Json -Depth 8),
  (New-Object System.Text.UTF8Encoding $false))
Write-Host ("Wrote {0} cases -> {1}" -f $summary.Count, $sumPath)
