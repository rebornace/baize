param([string]$tag = [guid]::NewGuid().ToString('N').Substring(0, 6))
$ErrorActionPreference = 'Stop'
$here = $PSScriptRoot
. (Join-Path $here 'common.ps1')
$cfg = Resolve-BaizeConfig
$BaseUrl = $cfg.BaseUrl
$ApiKey = $cfg.ApiKey

$cases = @(
  @{ id = 'users_page'; t = '帮我列出前5个用户' }
  @{ id = 'dashboard';  t = '给我看运营总览汇总数据' }
  @{ id = 'mall_orders'; t = '商城后台查询订单列表' }
  @{ id = 'pets_page';  t = '列出宠物，第一页10条' }
)

foreach ($c in $cases) {
  $run = Send-BaizeRun -BaseUrl $BaseUrl -ApiKey $ApiKey -AgentID 'ticket-agent' `
    -Text $c.t -ConversationID "fix-$tag-$($c.id)"
  $st = Wait-BaizeRun -BaseUrl $BaseUrl -ApiKey $ApiKey -RunID $run.run_id
  $d = Get-RunTokenEstimates -BaseUrl $BaseUrl -ApiKey $ApiKey -RunID $run.run_id
  $avgRatio = [math]::Round((($d | Measure-Object ratio -Average).Average), 3)
  Write-Host ("  {0,-12} status={1} steps={2} avg_ratio={3}" -f $c.id, $st, $d.Count, $avgRatio)
}
