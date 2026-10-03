# Offline analysis over a prior sweep output: prints (1) the per-case tier
# matrix (OK / alt / FAIL at each width) and (2) the aggregate success/cost
# report. Makes no network calls.
param(
  [string]$SweepDir = (Join-Path $PSScriptRoot 'out\sweep'),
  [int[]]$TopKs     = @(32, 24, 16, 12, 8),
  # Baseline average prompt used to compute relative savings in the report.
  [int]$BaselinePrompt = 0
)
$tiers = @{}
$present = @()
foreach ($k in $TopKs) {
  $f = Join-Path $SweepDir "topk_$k\summary.json"
  if (-not (Test-Path $f)) { continue }
  $parsed = Get-Content $f -Raw -Encoding UTF8 | ConvertFrom-Json
  $arr = @($parsed)
  $h = @{}
  foreach ($x in $arr) { $h[$x.id.ToString()] = $x }
  $tiers[$k] = $h
  $present += $k
}
if ($present.Count -eq 0) { throw "No tier summaries found under $SweepDir" }

# Baseline for relative savings: explicit arg, else widest present tier.
if ($BaselinePrompt -le 0) {
  $widest = ($present | Measure-Object -Maximum).Maximum
  $parsedB = Get-Content (Join-Path $SweepDir "topk_$widest\summary.json") -Raw -Encoding UTF8 | ConvertFrom-Json
  $BaselinePrompt = [math]::Round(((@($parsedB) | Measure-Object prompt_tokens -Average).Average), 0)
}

# (1) Per-case matrix.
$ids = @($tiers[$present[0]].Keys | Sort-Object)
$rows = foreach ($id in $ids) {
  $o = [ordered]@{ id = $id }
  foreach ($k in $present) {
    $c = $tiers[$k][$id]
    if ($c.status -eq 'succeeded' -and $c.expected_called) { $v = 'OK' }
    elseif ($c.status -eq 'succeeded') { $v = 'alt' }
    else { $v = 'FAIL' }
    $o["k$k"] = $v
  }
  [pscustomobject]$o
}
"=== per-case matrix (OK exact / alt acceptable alternative / FAIL hard fail) ==="
$rows | Format-Table -AutoSize | Out-String -Width 160

# (2) Aggregate cost/success report.
"=== aggregate (baseline avg prompt $BaselinePrompt) ==="
$report = foreach ($k in $present) {
  $parsed = Get-Content (Join-Path $SweepDir "topk_$k\summary.json") -Raw -Encoding UTF8 | ConvertFrom-Json
  $a = @($parsed)
  $n = $a.Count
  $exact = @($a | Where-Object { $_.status -eq 'succeeded' -and $_.expected_called }).Count
  $alt   = @($a | Where-Object { $_.status -eq 'succeeded' -and -not $_.expected_called }).Count
  $hard  = @($a | Where-Object { $_.status -ne 'succeeded' }).Count
  $avgPrompt = [math]::Round((($a | Measure-Object prompt_tokens -Average).Average), 0)
  $save = if ($BaselinePrompt -gt 0) { [math]::Round(100 * (1 - $avgPrompt / $BaselinePrompt), 0) } else { 0 }
  [pscustomobject]@{
    pre_topk         = $k
    exact            = $exact
    acceptable_alt   = $alt
    hard_fail        = $hard
    run_succeeded    = $n - $hard
    avg_sent         = [math]::Round((($a | Measure-Object sent_count -Average).Average), 1)
    avg_prompt       = $avgPrompt
    prompt_save      = "$save%"
  }
}
$report | Format-Table -AutoSize | Out-String -Width 150
