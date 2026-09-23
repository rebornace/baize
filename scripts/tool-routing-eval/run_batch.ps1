# Run every request in a dataset once and record status, whether the expected
# tool was called, per-turn prefilter recall, and turn-0 cost (sent tool count
# + prompt/completion tokens). Writes summary.json plus per-case <id>.events.json.
param(
  [string]$RequestsFile = (Join-Path $PSScriptRoot 'requests.json'),
  [string]$OutDir       = (Join-Path $PSScriptRoot 'out\runs'),
  [string]$AgentId      = 'miao-agent',
  [string]$BaseUrl      = '',
  [string]$ApiKey       = '',
  [int]$TimeoutSec      = 90
)
. (Join-Path $PSScriptRoot 'common.ps1')
$cfg = Resolve-BaizeConfig -BaseUrl $BaseUrl -ApiKey $ApiKey
$base = $cfg.BaseUrl
$key  = $cfg.ApiKey

New-Item -ItemType Directory -Force -Path $OutDir | Out-Null
$h = @{ Authorization = "Bearer $key" }
$parsedReqs = Get-Content $RequestsFile -Raw -Encoding UTF8 | ConvertFrom-Json
$reqs = @($parsedReqs)
$results = @()

foreach ($r in $reqs) {
  $body = @{ agent_id = $AgentId; input = $r.input } | ConvertTo-Json -Compress
  $tmpReq = Join-Path $OutDir ($r.id + '.req.json')
  [System.IO.File]::WriteAllText($tmpReq, $body, (New-Object System.Text.UTF8Encoding $false))
  $createRaw = curl.exe -s -X POST "$base/v0/runs" -H "Authorization: Bearer $key" `
    -H 'Content-Type: application/json' --data-binary "@$tmpReq"
  $create = $createRaw | ConvertFrom-Json
  $rid = $create.run_id

  $deadline = (Get-Date).AddSeconds($TimeoutSec)
  $status = 'running'
  while ((Get-Date) -lt $deadline) {
    Start-Sleep -Milliseconds 1500
    $g = Invoke-RestMethod -Uri "$base/v0/runs/$rid" -Headers $h -TimeoutSec 10
    $status = $g.status
    if ($status -in @('succeeded', 'failed', 'cancelled')) { break }
  }

  $evFile = Join-Path $OutDir ($r.id + '.events.json')
  curl.exe -s "$base/v0/runs/$rid/events" -H "Authorization: Bearer $key" -o $evFile
  # NB PS5: wrapping ConvertFrom-Json directly in @() keeps the emitted array as
  # a single element; assign first, then wrap on use.
  $parsedEvs = Get-Content $evFile -Raw -Encoding UTF8 | ConvertFrom-Json
  $evs = @($parsedEvs)

  # Group tool calls by turn using narrow events as boundaries.
  $turnOfCall = @{}
  $curTurn = 0
  foreach ($ev in $evs) {
    if ($ev.type -eq 'decide.tool_narrow') {
      $curTurn = [int]$ev.data.turn
    } elseif ($ev.type -eq 'llm.tool_call') {
      $turnOfCall[$ev.data.id] = $curTurn
    }
  }
  $narrows = @($evs | Where-Object { $_.type -eq 'decide.tool_narrow' })
  $toolCalls = @($evs | Where-Object { $_.type -eq 'llm.tool_call' } | ForEach-Object { $_.data.name })

  $preHit = $true
  $preDetail = @()
  foreach ($ev in ($evs | Where-Object { $_.type -eq 'llm.tool_call' })) {
    $name = $ev.data.name
    $t = $turnOfCall[$ev.data.id]
    $sh = $narrows | Where-Object { [int]$_.data.turn -eq $t } | Select-Object -First 1
    if ($sh) {
      $inPre = @($sh.data.prefilter) -contains $name
      if (-not $inPre) { $preHit = $false }
      $preDetail += "$name@turn${t}:" + $(if ($inPre) { 'in' } else { 'MISS' })
    }
  }

  $expectMet = $false
  foreach ($want in $r.expect_any) {
    if ($toolCalls -match [regex]::Escape($want)) { $expectMet = $true }
  }
  $shT0 = $narrows | Select-Object -First 1
  $uT0 = $evs | Where-Object { $_.type -eq 'llm.usage' } | Select-Object -First 1
  $results += [pscustomobject]@{
    id               = $r.id
    status           = $status
    expected_called  = $expectMet
    prefilter_recall = $preHit
    sent_count       = $shT0.data.sent_count
    prompt_tokens    = $uT0.data.prompt_tokens
    completion_tokens = $uT0.data.completion_tokens
    calls            = ($toolCalls -join '>')
    detail           = ($preDetail -join '; ')
  }
  Write-Host ("[{0}] status={1} expect_called={2} prefilter_recall={3}" -f $r.id, $status, $expectMet, $preHit)
}

$results | ConvertTo-Json -Depth 4 | Out-File (Join-Path $OutDir 'summary.json') -Encoding utf8
$results | Format-Table -AutoSize | Out-String -Width 220
