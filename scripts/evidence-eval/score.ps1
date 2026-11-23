# Offline score: case + events JSON → pass/fail with SafeAct-style codes.
param(
  [Parameter(Mandatory = $true)][string]$CaseFile,
  [Parameter(Mandatory = $true)][string]$EventsFile,
  [string]$CaseId = '',
  [string]$RunStatus = '',
  [string]$StateFile = ''
)
. (Join-Path $PSScriptRoot 'common.ps1')

$parsedCase = Get-Content $CaseFile -Raw -Encoding UTF8 | ConvertFrom-Json
$caseList = @($parsedCase)  # assign-then-wrap (PS5 nested-array pitfall)
if ($CaseId) {
  $case = $caseList | Where-Object { [string]$_.id -eq $CaseId } | Select-Object -First 1
  if (-not $case) { throw "CaseId not found: $CaseId" }
} elseif ($caseList.Count -eq 1) {
  $case = $caseList[0]
} else {
  throw 'CaseFile has multiple cases; pass -CaseId.'
}

$parsed = Get-Content $EventsFile -Raw -Encoding UTF8 | ConvertFrom-Json
$evs = @($parsed)
$tl = Get-ToolTimeline -Events $evs
$stateSnap = $null
if ($StateFile) {
  $stateSnap = Get-Content $StateFile -Raw -Encoding UTF8 | ConvertFrom-Json
} else {
  $autoState = ($EventsFile -replace '\.events\.json$','') + '.state.json'
  if (Test-Path -LiteralPath $autoState) {
    $stateSnap = Get-Content $autoState -Raw -Encoding UTF8 | ConvertFrom-Json
  }
}
$result = Score-EvidenceCase -Case $case -Timeline $tl -RunStatus $RunStatus -StateSnapshot $stateSnap
$result | ConvertTo-Json -Depth 5 -Compress
if (-not $result.pass) { exit 1 }
