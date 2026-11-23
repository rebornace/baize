# Soft-gate A/B for SSR1 KindToolPick (baseline mode=off vs live).
# Copies compare.json + arm summaries to docs/superpowers/notes/eval-artifacts/ssr1-tool-pick/<ts>/.
# TIE (delta==0): retest allowed; stop after MaxTies cumulative TIE folders (default 5).
param(
  [string]$RequestsFile = (Join-Path $PSScriptRoot 'requests.json'),
  [string]$AgentId      = 'default-agent',
  [string]$BaseUrl      = '',
  [string]$ApiKey       = '',
  [int]$TimeoutSec      = 90,
  [int]$TopK            = 16,
  [int]$MaxTies         = 5
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')
$cfg = Resolve-BaizeConfig -BaseUrl $BaseUrl -ApiKey $ApiKey
$base = $cfg.BaseUrl
$key  = $cfg.ApiKey

$artifactRoot = Join-Path $PSScriptRoot '..\..\docs\superpowers\notes\eval-artifacts\ssr1-tool-pick'
# Count prior TIE conclusions so we do not burn tokens forever.
$priorTies = 0
if (Test-Path $artifactRoot) {
  Get-ChildItem -Path $artifactRoot -Directory -ErrorAction SilentlyContinue | ForEach-Object {
    $c = Join-Path $_.FullName 'CONCLUSION.md'
    if (Test-Path $c) {
      $text = [System.IO.File]::ReadAllText($c)
      if ($text -match '(?m)^\s*##\s*Verdict:\s*\*\*TIE\*\*') { $priorTies++ }
    }
  }
}
if ($priorTies -ge $MaxTies) {
  Write-Host ("Already have {0} TIE run(s) (MaxTies={1}). Stop retesting to save tokens. Decide manually." -f $priorTies, $MaxTies)
  exit 2
}
Write-Host ("Prior TIE runs: {0} / MaxTies {1}" -f $priorTies, $MaxTies)

$ts = Get-Date -Format 'yyyyMMdd-HHmmss'
$workRoot = Join-Path $PSScriptRoot ("out\ssr1-tool-pick-{0}" -f $ts)
New-Item -ItemType Directory -Force -Path $workRoot | Out-Null

function Invoke-Arm {
  param([string]$Mode, [string]$ArmDir)
  New-Item -ItemType Directory -Force -Path $ArmDir | Out-Null
  $body = (@{
    decide_tool_routing_enabled = $true
    decide_tool_pre_topk        = $TopK
    decide_tool_pick_mode       = $Mode
  } | ConvertTo-Json -Compress)
  Set-RuntimeKnobs -BaseUrl $base -ApiKey $key -Json $body
  & (Join-Path $PSScriptRoot 'run_batch.ps1') `
    -RequestsFile $RequestsFile -OutDir $ArmDir -AgentId $AgentId `
    -BaseUrl $base -ApiKey $key -TimeoutSec $TimeoutSec
}

$baselineDir = Join-Path $workRoot 'baseline'
$liveDir     = Join-Path $workRoot 'live'
Write-Host "=== Arm A baseline (decide_tool_pick_mode=off) ==="
Invoke-Arm -Mode 'off' -ArmDir $baselineDir
Write-Host "=== Arm B live (decide_tool_pick_mode=live) ==="
Invoke-Arm -Mode 'live' -ArmDir $liveDir

function Read-Summary([string]$path) {
  $p = Join-Path $path 'summary.json'
  if (-not (Test-Path $p)) { throw "missing $p" }
  $raw = [System.IO.File]::ReadAllText($p)
  return ($raw | ConvertFrom-Json)
}

$baseSum = @(Read-Summary $baselineDir)
$liveSum  = @(Read-Summary $liveDir)

function Summarize($rows) {
  $n = $rows.Count
  $hit = @($rows | Where-Object { $_.expected_called }).Count
  $ok  = @($rows | Where-Object { $_.status -eq 'succeeded' }).Count
  $wait = @($rows | Where-Object { $_.status -eq 'waiting_human' }).Count
  return [ordered]@{
    n               = $n
    expected_hit    = $hit
    run_succeeded   = $ok
    waiting_human   = $wait
  }
}

$b = Summarize $baseSum
$l = Summarize $liveSum
$delta = [int]$l.expected_hit - [int]$b.expected_hit

$improved = @()
$regressed = @()
$byId = @{}
foreach ($r in $baseSum) { $byId[$r.id] = @{ baseline = $r } }
foreach ($r in $liveSum) {
  if (-not $byId.ContainsKey($r.id)) { $byId[$r.id] = @{} }
  $byId[$r.id].live = $r
}
foreach ($id in $byId.Keys) {
  $pair = $byId[$id]
  if (-not $pair.baseline -or -not $pair.live) { continue }
  $bh = [bool]$pair.baseline.expected_called
  $lh = [bool]$pair.live.expected_called
  if (-not $bh -and $lh) { $improved += $id }
  if ($bh -and -not $lh) { $regressed += $id }
}

# Soft-gate:
#   PASS  = live expected_hit >= baseline + 1
#   TIE   = delta == 0  → retest; do NOT discard yet
#   FAIL  = live worse than baseline (delta < 0)
if ($delta -ge 1) {
  $verdict = 'PASS'
} elseif ($delta -eq 0) {
  $verdict = 'TIE'
} else {
  $verdict = 'FAIL'
}
$pass = ($verdict -eq 'PASS')
$tie = ($verdict -eq 'TIE')

$compare = [ordered]@{
  schema_version = 1
  timestamp      = $ts
  base_url       = $base
  agent_id       = $AgentId
  knobs          = [ordered]@{
    decide_tool_routing_enabled = $true
    decide_tool_pre_topk        = $TopK
    arms = [ordered]@{
      baseline = @{ decide_tool_pick_mode = 'off' }
      live     = @{ decide_tool_pick_mode = 'live' }
    }
  }
  overall = [ordered]@{
    baseline           = $b
    live               = $l
    delta_expected_hit = $delta
    improved_cases     = @($improved)
    regressed_cases    = @($regressed)
  }
  soft_gate = [ordered]@{
    rule    = 'PASS if delta>=1; TIE if delta==0 (retest); FAIL if delta<0'
    verdict = $verdict
  }
}

$comparePath = Join-Path $workRoot 'compare.json'
$compare | ConvertTo-Json -Depth 8 | Set-Content -Path $comparePath -Encoding utf8

$artifactDir = Join-Path $artifactRoot $ts
$tieCountAfter = if ($tie) { $priorTies + 1 } else { $priorTies }
New-Item -ItemType Directory -Force -Path $artifactDir | Out-Null
Copy-Item $comparePath (Join-Path $artifactDir 'compare.json') -Force
Copy-Item (Join-Path $baselineDir 'summary.json') (Join-Path $artifactDir 'baseline-summary.json') -Force
Copy-Item (Join-Path $liveDir 'summary.json') (Join-Path $artifactDir 'live-summary.json') -Force

$conclusion = @"
# SSR1 tool-pick soft-gate — $ts

## Verdict: **$verdict**

| Arm | n | expected_hit | run_succeeded | waiting_human |
|-----|---|--------------|---------------|---------------|
| baseline (pick off) | $($b.n) | **$($b.expected_hit)** | $($b.run_succeeded) | $($b.waiting_human) |
| live (pick live) | $($l.n) | **$($l.expected_hit)** | $($l.run_succeeded) | $($l.waiting_human) |
| delta | | **$delta** | | |

- Improved: $($improved -join ', ')
- Regressed: $($regressed -join ', ')

## Artifacts

- Worktree: $workRoot
- Committed-path copy: docs/superpowers/notes/eval-artifacts/ssr1-tool-pick/$ts/

## Next (spec)

$(if ($pass) {
  'PASS: set decide_tool_pick_mode default to live; mark menu/backlog delivered; no user settings page.'
} elseif ($tie) {
  if ($tieCountAfter -ge $MaxTies) {
    "TIE #${tieCountAfter} / ${MaxTies}: stop retesting (token budget). Do NOT discard automatically; human decides keep / discard / change gate."
  } else {
    "TIE #${tieCountAfter} / ${MaxTies}: do NOT discard. Re-run compare_tool_pick.ps1; stop after ${MaxTies} TIEs."
  }
} else {
  'FAIL: discard feature code only after human confirm; keep these artifacts; menu mark 否决.'
})
"@
[System.IO.File]::WriteAllText((Join-Path $artifactDir 'CONCLUSION.md'), $conclusion)

Write-Host ""
Write-Host "soft-gate verdict=$verdict delta_expected_hit=$delta tie_count=$tieCountAfter/$MaxTies"
Write-Host "artifacts: $artifactDir"
Write-Host "Review compare.json then apply PASS / TIE-retest / FAIL (confirm before discard)."
if ($tie -and $tieCountAfter -ge $MaxTies) {
  Write-Host "MaxTies reached — further compare_tool_pick.ps1 runs will exit 2 until you clear or decide."
}
