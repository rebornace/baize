# Sweep decide_tool_pre_topk: for each requested width, hot patch the knob and
# run the full dataset once, then aggregate success rate / average sent tools
# / average prompt tokens. Restores the knobs that were in effect before the
# run, even on interruption (trap).
param(
  [int[]]$TopKs        = @(32, 24, 16, 12, 8),
  [string]$RequestsFile = (Join-Path $PSScriptRoot 'requests.json'),
  [string]$OutRoot      = (Join-Path $PSScriptRoot 'out\sweep'),
  [string]$AgentId      = 'miao-agent',
  [string]$BaseUrl      = '',
  [string]$ApiKey       = ''
)
. (Join-Path $PSScriptRoot 'common.ps1')
$ErrorActionPreference = 'Continue'
$cfg = Resolve-BaizeConfig -BaseUrl $BaseUrl -ApiKey $ApiKey
$base = $cfg.BaseUrl
$key  = $cfg.ApiKey
$batch = Join-Path $PSScriptRoot 'run_batch.ps1'

New-Item -ItemType Directory -Force -Path $OutRoot | Out-Null

# Capture pre-existing effective knobs to restore them afterwards.
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

# Tool narrowing is now a single mode (no observe-only): turn it on.
Set-RuntimeKnobs -BaseUrl $base -ApiKey $key -Json '{"decide_enabled":true,"decide_tool_routing_enabled":true}'

foreach ($k in $TopKs) {
  Set-RuntimeKnobs -BaseUrl $base -ApiKey $key -Json ("{`"decide_tool_pre_topk`":$k}")
  Write-Host ("### pre_topk={0} ###" -f $k)
  powershell -ExecutionPolicy Bypass -File $batch `
    -RequestsFile $RequestsFile -OutDir (Join-Path $OutRoot "topk_$k") `
    -AgentId $AgentId -BaseUrl $base -ApiKey $key | Out-Null
}

& $restore

# Aggregate.
$agg = foreach ($k in $TopKs) {
  $parsed = Get-Content (Join-Path $OutRoot "topk_$k\summary.json") -Raw -Encoding UTF8 | ConvertFrom-Json
  $a = @($parsed)
  $n = $a.Count
  $ok = @($a | Where-Object { $_.expected_called -eq $true -and $_.status -eq 'succeeded' }).Count
  $hard = @($a | Where-Object { $_.status -ne 'succeeded' }).Count
  [pscustomobject]@{
    pre_topk      = $k
    success       = "$ok/$n"
    rate          = [math]::Round(100 * $ok / $n, 1)
    run_succeeded = $n - $hard
    avg_sent      = [math]::Round((($a | Measure-Object sent_count -Average).Average), 1)
    avg_prompt    = [math]::Round((($a | Measure-Object prompt_tokens -Average).Average), 0)
    avg_completion = [math]::Round((($a | Measure-Object completion_tokens -Average).Average), 0)
  }
}
$agg | Format-Table -AutoSize | Out-String -Width 160
$agg | ConvertTo-Json | Out-File (Join-Path $OutRoot 'aggregate.json') -Encoding utf8
