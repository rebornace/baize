# Summarize a layer-B out dir: summary.json + per-case events for systems_source.
param(
  [Parameter(Mandatory = $true)][string]$OutDir,
  [string]$Label = ''
)
$ErrorActionPreference = 'Continue'
if (-not $Label) { $Label = Split-Path $OutDir -Leaf }
$sumPath = Join-Path $OutDir 'summary.json'
if (-not (Test-Path $sumPath)) { throw "missing $sumPath" }
$parsed = Get-Content $sumPath -Raw -Encoding UTF8 | ConvertFrom-Json
$rows = @($parsed)
$n = $rows.Count
$ok = @($rows | Where-Object { $_.status -eq 'succeeded' }).Count
$expect = @($rows | Where-Object { $_.expected_called }).Count
$pre = @($rows | Where-Object { $_.prefilter_recall }).Count
$avgPrompt = [math]::Round((($rows | Where-Object { $_.prompt_tokens } | Measure-Object prompt_tokens -Average).Average), 0)
$avgSent = [math]::Round((($rows | Where-Object { $_.sent_count } | Measure-Object sent_count -Average).Average), 1)

$srcCount = @{}
$deg = 0
Get-ChildItem $OutDir -Filter '*.events.json' | ForEach-Object {
  $evs = @(Get-Content $_.FullName -Raw -Encoding UTF8 | ConvertFrom-Json)
  foreach ($ev in $evs) {
    if ($ev.type -eq 'decide.tool_narrow') {
      $s = [string]$ev.data.systems_source
      if (-not $s) { $s = '(empty)' }
      if (-not $srcCount.ContainsKey($s)) { $srcCount[$s] = 0 }
      $srcCount[$s]++
      if ($ev.data.systems_degraded) { $deg++ }
    }
  }
}
$wall = $null
$wallFile = Join-Path $OutDir 'wall_seconds.txt'
if (Test-Path $wallFile) { $wall = [double]((Get-Content $wallFile -Raw).Trim()) }

[pscustomobject]@{
  label              = $Label
  n                  = $n
  run_succeeded      = $ok
  expected_called    = $expect
  prefilter_recall   = $pre
  avg_prompt_tokens  = $avgPrompt
  avg_sent_count     = $avgSent
  systems_sources    = ($srcCount.GetEnumerator() | ForEach-Object { "$($_.Key)=$($_.Value)" }) -join '; '
  systems_degraded_n = $deg
  wall_seconds       = $wall
} | Format-List
