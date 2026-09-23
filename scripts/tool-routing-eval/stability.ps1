# Repeatedly run the full dataset at one fixed pre_topk to measure stability:
# per-round success/hard-fail and per-case flakiness (how often each case was
# exact / acceptable-alternative / failed). Restores the knobs that were in
# effect before the run.
param(
  [int]$Rounds         = 5,
  [int]$TopK           = 16,
  [string]$RequestsFile = (Join-Path $PSScriptRoot 'requests.json'),
  [string]$OutRoot      = '',
  [string]$AgentId      = 'miao-agent',
  [string]$BaseUrl      = '',
  [string]$ApiKey       = ''
)
. (Join-Path $PSScriptRoot 'common.ps1')
$ErrorActionPreference = 'Continue'
$cfg = Resolve-BaizeConfig -BaseUrl $BaseUrl -ApiKey $ApiKey
$base = $cfg.BaseUrl
$key  = $cfg.ApiKey
if (-not $OutRoot) { $OutRoot = Join-Path $PSScriptRoot ("out\stab_$TopK") }
$batch = Join-Path $PSScriptRoot 'run_batch.ps1'

New-Item -ItemType Directory -Force -Path $OutRoot | Out-Null

$before = Invoke-RestMethod "$base/v0/settings/runtime" -Headers @{ Authorization = "Bearer $key" }
$restoreEnabled = [bool]$before.effective.decide_enabled
$restoreRouting = [bool]$before.effective.decide_tool_routing_enabled
$restoreTopK    = [int]$before.effective.decide_tool_pre_topk
$restore = {
  Set-RuntimeKnobs -BaseUrl $base -ApiKey $key -Json `
    ("{{`"decide_enabled`":{0},`"decide_tool_routing_enabled`":{1},`"decide_tool_pre_topk`":{2}}}" `
      -f $(if ($restoreEnabled) { 'true' } else { 'false' }), $(if ($restoreRouting) { 'true' } else { 'false' }), $restoreTopK)
  Write-Host ("### RESTORED decide={0} routing={1} pre_topk={2} ###" -f $restoreEnabled, $restoreRouting, $restoreTopK)
}
trap { & $restore; break }

# Tool narrowing is a single mode (no observe-only): turn it on at this width.
Set-RuntimeKnobs -BaseUrl $base -ApiKey $key -Json `
  ("{`"decide_enabled`":true,`"decide_tool_routing_enabled`":true,`"decide_tool_pre_topk`":$TopK}")

for ($i = 1; $i -le $Rounds; $i++) {
  Write-Host ("### Round {0}/{1} (pre_topk={2}) ###" -f $i, $Rounds, $TopK)
  powershell -ExecutionPolicy Bypass -File $batch `
    -RequestsFile $RequestsFile -OutDir (Join-Path $OutRoot "round_$i") `
    -AgentId $AgentId -BaseUrl $base -ApiKey $key | Out-Null
}

& $restore

# Per-round table (note: distinct name from $Rounds to avoid clobbering).
$roundStats = for ($i = 1; $i -le $Rounds; $i++) {
  $parsed = Get-Content (Join-Path $OutRoot "round_$i\summary.json") -Raw -Encoding UTF8 | ConvertFrom-Json
  $a = @($parsed)
  [pscustomobject]@{
    round         = $i
    exact         = @($a | Where-Object { $_.status -eq 'succeeded' -and $_.expected_called }).Count
    hard_fail     = @($a | Where-Object { $_.status -ne 'succeeded' }).Count
    run_succeeded = @($a | Where-Object { $_.status -eq 'succeeded' }).Count
    avg_prompt    = [math]::Round((($a | Measure-Object prompt_tokens -Average).Average), 0)
  }
}
$roundStats | Format-Table -AutoSize | Out-String -Width 120
$roundStats | ConvertTo-Json | Out-File (Join-Path $OutRoot 'rounds.json') -Encoding utf8

# Per-case flakiness across rounds.
$caseStat = @{}
for ($i = 1; $i -le $Rounds; $i++) {
  $parsed = Get-Content (Join-Path $OutRoot "round_$i\summary.json") -Raw -Encoding UTF8 | ConvertFrom-Json
  foreach ($x in @($parsed)) {
    if (-not $caseStat.ContainsKey($x.id)) { $caseStat[$x.id] = @{ ok = 0; alt = 0; fail = 0 } }
    if ($x.status -ne 'succeeded') { $caseStat[$x.id].fail++ }
    elseif ($x.expected_called) { $caseStat[$x.id].ok++ }
    else { $caseStat[$x.id].alt++ }
  }
}
$flaky = foreach ($id in ($caseStat.Keys | Sort-Object)) {
  $s = $caseStat[$id]
  [pscustomobject]@{ id = $id; ok = $s.ok; alt = $s.alt; fail = $s.fail }
}
$flaky | ConvertTo-Json | Out-File (Join-Path $OutRoot 'cases.json') -Encoding utf8
$unstable = @($flaky | Where-Object { $_.fail -gt 0 -or $_.alt -gt 0 })
"unstable count=$($unstable.Count) of $($flaky.Count)"
$unstable | Format-Table -AutoSize | Out-String -Width 120
