# Dual-arm A/B for CX2 fact retention (baseline offload off vs live on).
# Copies compare.json + arm summaries + CONCLUSION.md to
# docs/superpowers/notes/eval-artifacts/cx2-fact-retention/<ts>/.
# Soft-close = human verdict only; script never changes product defaults.
param(
  [string]$RequestsFile = (Join-Path $PSScriptRoot 'requests.json'),
  [string]$AgentId      = 'default-agent',
  [string]$BaseUrl      = '',
  [string]$ApiKey       = '',
  [int]$TimeoutSec      = 180
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$cfg = Resolve-BaizeConfig -BaseUrl $BaseUrl -ApiKey $ApiKey
$base = $cfg.BaseUrl; $key = $cfg.ApiKey

$ts = Get-Date -Format 'yyyyMMdd-HHmmss'
$workRoot = Join-Path $PSScriptRoot ("out\cx2-fact-retention-{0}" -f $ts)
New-Item -ItemType Directory -Force -Path $workRoot | Out-Null

function Invoke-Arm([bool]$OffloadOn, [string]$ArmDir, [string]$Label) {
  New-Item -ItemType Directory -Force -Path $ArmDir | Out-Null
  $body = (@{ context_offload_enabled = $OffloadOn } | ConvertTo-Json -Compress)
  Set-RuntimeKnobs -BaseUrl $base -ApiKey $key -Json $body
  Assert-OffloadKnob -BaseUrl $base -ApiKey $key -Expected $OffloadOn
  if ($OffloadOn) { $env:CX2_EXPECT_OFFLOAD = '1' } else { $env:CX2_EXPECT_OFFLOAD = '0' }
  Write-Host "=== Arm $Label offload=$OffloadOn ==="
  & (Join-Path $PSScriptRoot 'run_batch.ps1') `
    -RequestsFile $RequestsFile -OutDir $ArmDir -AgentId $AgentId `
    -BaseUrl $base -ApiKey $key -TimeoutSec $TimeoutSec
}

$baselineDir = Join-Path $workRoot 'baseline'
$liveDir     = Join-Path $workRoot 'live'

function RetentionRate($rows) {
  $all = @($rows)
  $valid = @($all | Where-Object { -not $_.invalid })
  $hit = 0; $tot = 0
  foreach ($r in $valid) { $hit += [int]$r.hits; $tot += [int]$r.total_q }
  $rate = if ($tot -gt 0) { [math]::Round($hit / $tot, 4) } else { $null }
  return [ordered]@{
    n = @($all).Count
    valid = @($valid).Count
    invalid = @($all | Where-Object { $_.invalid }).Count
    hits = $hit
    total_q = $tot
    retention_rate = $rate
  }
}

try {
  Invoke-Arm -OffloadOn $false -ArmDir $baselineDir -Label 'baseline'
  Invoke-Arm -OffloadOn $true  -ArmDir $liveDir     -Label 'live'

  # PS5: assign ConvertFrom-Json first, then wrap — inline @() collapses single-element arrays.
  $baseParsed = Get-Content (Join-Path $baselineDir 'summary.json') -Raw -Encoding UTF8 | ConvertFrom-Json
  $liveParsed = Get-Content (Join-Path $liveDir 'summary.json') -Raw -Encoding UTF8 | ConvertFrom-Json
  $baseRows = @($baseParsed)
  $liveRows = @($liveParsed)
  $b = RetentionRate $baseRows
  $l = RetentionRate $liveRows
  $delta = $null
  if ($null -ne $b.retention_rate -and $null -ne $l.retention_rate) {
    $delta = [math]::Round(([double]$l.retention_rate - [double]$b.retention_rate), 4)
  }

  $compare = [ordered]@{
    ts = $ts
    baseline = $b
    live = $l
    delta_retention = $delta
  }
  $comparePath = Join-Path $workRoot 'compare.json'
  [System.IO.File]::WriteAllText($comparePath, ($compare | ConvertTo-Json -Depth 6),
    (New-Object System.Text.UTF8Encoding $false))

  $artifactDir = Join-Path $PSScriptRoot "..\..\docs\superpowers\notes\eval-artifacts\cx2-fact-retention\$ts"
  New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
  Copy-Item $comparePath $artifactDir
  Copy-Item (Join-Path $baselineDir 'summary.json') (Join-Path $artifactDir 'baseline-summary.json')
  Copy-Item (Join-Path $liveDir 'summary.json') (Join-Path $artifactDir 'live-summary.json')

  $conclusion = @"
# CX2 fact-retention — $ts

## Verdict: **(human)** — fill after reading rates

| Arm | valid | invalid | hits/total_q | retention_rate |
|-----|-------|---------|--------------|----------------|
| baseline (offload off) | $($b.valid) | $($b.invalid) | $($b.hits)/$($b.total_q) | $($b.retention_rate) |
| live (offload on) | $($l.valid) | $($l.invalid) | $($l.hits)/$($l.total_q) | $($l.retention_rate) |
| delta (live - baseline) | | | | $delta |

## Soft-close

Report only — no auto discard / no default knob change (spec closeout=1).

## Artifacts

- Worktree: ``$workRoot``
- This folder: compare.json, baseline-summary.json, live-summary.json
"@
  [System.IO.File]::WriteAllText((Join-Path $artifactDir 'CONCLUSION.md'), $conclusion,
    (New-Object System.Text.UTF8Encoding $false))
  Write-Host "Archived -> $artifactDir"
  Write-Host ("delta_retention={0} baseline_rate={1} live_rate={2}" -f $delta, $b.retention_rate, $l.retention_rate)
} finally {
  # Always restore CX1 default offload-on, even if Assert / run_batch / archive fails.
  try {
    Set-RuntimeKnobs -BaseUrl $base -ApiKey $key -Json '{"context_offload_enabled":true}'
  } catch {
    Write-Host ("WARN: failed to restore context_offload_enabled=true: {0}" -f $_)
  }
  $env:CX2_EXPECT_OFFLOAD = '0'
}
