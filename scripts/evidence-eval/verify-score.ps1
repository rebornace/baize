# Offline regression for Score-EvidenceCase (no server, no API key).
# Expectation file names: <caseId>.<tag>.events.json where tag is pass|par|bsr|gap|...
param(
  [string]$FixturesDir = (Join-Path $PSScriptRoot 'fixtures'),
  [string]$CasesFile   = (Join-Path $PSScriptRoot 'cases.json')
)
$ErrorActionPreference = 'Stop'
. (Join-Path $PSScriptRoot 'common.ps1')

# PS5: assign ConvertFrom-Json first, then @() — wrapping the pipeline nests the array.
$parsedCases = Get-Content $CasesFile -Raw -Encoding UTF8 | ConvertFrom-Json
$cases = @($parsedCases)
$byId = @{}
foreach ($c in $cases) { $byId[[string]$c.id] = $c }

$failed = 0
$ran = 0
Get-ChildItem -Path $FixturesDir -Filter '*.events.json' | Sort-Object Name | ForEach-Object {
  # File name is <caseId>.<tag>.events.json — BaseName alone leaves ".events".
  $fname = $_.Name
  if ($fname -notmatch '^(?<id>.+)\.(?<tag>[^.]+)\.events\.json$') {
    Write-Host "SKIP odd fixture: $fname"
    return
  }
  $id = $Matches['id']
  $tag = $Matches['tag'].ToLowerInvariant()
  if (-not $byId.ContainsKey($id)) {
    Write-Host "FAIL $($_.Name): unknown case id $id"
    $failed++
    $ran++
    return
  }
  $parsedEvs = Get-Content $_.FullName -Raw -Encoding UTF8 | ConvertFrom-Json
  $evs = @($parsedEvs)
  $tl = Get-ToolTimeline -Events $evs
  $result = Score-EvidenceCase -Case $byId[$id] -Timeline $tl
  $ran++
  $expectPass = ($tag -eq 'pass')
  if ($expectPass -ne [bool]$result.pass) {
    Write-Host ("FAIL {0}: expect pass={1} got pass={2} codes={3} detail={4}" -f `
      $fname, $expectPass, $result.pass, ($result.codes -join ','), $result.detail)
    $failed++
    return
  }
  if (-not $expectPass) {
    # Tag should appear among failure codes (par→PAR, gap→GAP, bsr→BSR).
    $wantCode = $tag.ToUpperInvariant()
    if (@($result.codes) -notcontains $wantCode) {
      Write-Host ("FAIL {0}: expected code {1} in [{2}]" -f $fname, $wantCode, ($result.codes -join ','))
      $failed++
      return
    }
  }
  Write-Host ("OK   {0} pass={1} codes=[{2}]" -f $fname, $result.pass, ($result.codes -join ','))
}

Write-Host ("verify-score: {0} fixtures, {1} failed" -f $ran, $failed)
if ($failed -gt 0) { exit 1 }
