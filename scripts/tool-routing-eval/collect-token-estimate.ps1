# Data collector for token-estimator bias (tokens.estimate).
# Runs a diverse set of independent requests plus one multi-turn conversation
# with tool narrowing on (pre_topk=16), then writes every collected
# tokens.estimate record as JSON for offline analysis.
#
# Usage:
#   .\collect-token-estimate.ps1
param(
  [string]$BaseUrl = 'http://127.0.0.1:8080',
  [string]$AgentID = 'miao-agent',
  [int]$PollIntervalSec = 2,
  [int]$PollTimeoutSec = 240
)

$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

$cfg = Resolve-BaizeConfig -BaseUrl $BaseUrl
$BaseUrl = $cfg.BaseUrl
$ApiKey = $cfg.ApiKey

$outDir = Join-Path $PSScriptRoot 'artifacts'
New-Item -ItemType Directory -Force $outDir | Out-Null

# Select a diverse subset (three systems + built-in, list & detail intents) to
# keep token spend bounded while still spanning the catalog.
$caseIDs = @(
  'users_page', 'user_detail', 'pets_page', 'pet_detail',
  'device_types', 'dashboard',
  'mall_orders', 'mall_order_det', 'mall_products', 'mall_sku',
  'notion_search', 'notion_recent'
)
$requests = Get-Content (Join-Path $PSScriptRoot 'requests.json') -Raw -Encoding UTF8 | ConvertFrom-Json
$cases = foreach ($id in $caseIDs) { $requests | Where-Object { $_.id -eq $id } }

# --- Enable tool narrowing (single mode; pre_topk=16) -----------------------
Set-RuntimeKnobs -BaseUrl $BaseUrl -ApiKey $ApiKey -Json `
  '{"decide_enabled":true,"decide_tool_routing_enabled":true,"decide_tool_pre_topk":16}'
Write-Host 'tool narrowing on (pre_topk=16)'

$records = @()
$tag = [guid]::NewGuid().ToString('N').Substring(0, 8)

# --- Independent cases ------------------------------------------------------
foreach ($c in $cases) {
  $conv = "est-$tag-$($c.id)"
  $run = Send-BaizeRun -BaseUrl $BaseUrl -ApiKey $ApiKey -AgentID $AgentID `
    -Text $c.input -ConversationID $conv
  $status = Wait-BaizeRun -BaseUrl $BaseUrl -ApiKey $ApiKey -RunID $run.run_id `
    -PollIntervalSec $PollIntervalSec -PollTimeoutSec $PollTimeoutSec
  $est = @(Get-RunTokenEstimates -BaseUrl $BaseUrl -ApiKey $ApiKey -RunID $run.run_id)
  Write-Host ("  {0,-16} status={1} estimates={2}" -f $c.id, $status, $est.Count)
  $i = 0
  foreach ($d in $est) {
    $records += [pscustomobject]@{
      kind = 'single'; case_id = $c.id; step = $i; status = $status; data = $d
    }
    $i++
  }
}

# --- Multi-turn conversation (history grows each turn) ----------------------
$turns = @(
  '帮我列出前5个用户',
  '查一下 id=1 的用户详情',
  '这个用户关联了哪些宠物',
  '列出宠物相关的设备',
  '设备类型都有哪些',
  '再给我看下运营总览汇总数据'
)
$mtConv = "est-$tag-multiturn"
for ($t = 0; $t -lt $turns.Count; $t++) {
  $run = Send-BaizeRun -BaseUrl $BaseUrl -ApiKey $ApiKey -AgentID $AgentID `
    -Text $turns[$t] -ConversationID $mtConv
  $status = Wait-BaizeRun -BaseUrl $BaseUrl -ApiKey $ApiKey -RunID $run.run_id `
    -PollIntervalSec $PollIntervalSec -PollTimeoutSec $PollTimeoutSec
  $est = @(Get-RunTokenEstimates -BaseUrl $BaseUrl -ApiKey $ApiKey -RunID $run.run_id)
  Write-Host ("  multiturn[{0}] status={1} estimates={2}" -f $t, $status, $est.Count)
  $i = 0
  foreach ($d in $est) {
    $records += [pscustomobject]@{
      kind = 'multiturn'; case_id = "turn$t"; step = $i; status = $status; data = $d
    }
    $i++
  }
}

$outFile = Join-Path $outDir 'estimates.json'
$records | ConvertTo-Json -Depth 8 | Set-Content $outFile -Encoding UTF8
Write-Host "wrote $($records.Count) records to $outFile"
