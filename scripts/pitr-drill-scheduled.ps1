# Runs the isolated PITR drill and writes a timestamped report. Optionally
# registers a daily Windows Scheduled Task that repeats the drill.
#
# Usage:
#   .\scripts\pitr-drill-scheduled.ps1
#   .\scripts\pitr-drill-scheduled.ps1 -Keep -ReportDir R:\Temp\opencode\pitr
#   .\scripts\pitr-drill-scheduled.ps1 -RegisterTask -DailyAt 03:00
param(
    [string]$ReportDir = "docs/superpowers/notes/eval-artifacts/pitr-drill-scheduled",
    [switch]$Keep,
    [string]$Docker = "",
    [switch]$RegisterTask,
    [string]$TaskName = "Baize PITR Drill",
    [string]$DailyAt = "03:00"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$drill = Join-Path $PSScriptRoot "pitr-drill.ps1"
if (-not (Test-Path -LiteralPath $drill)) {
    throw "drill script not found: $drill"
}
$reportPath = if ([System.IO.Path]::IsPathRooted($ReportDir)) {
    $ReportDir
} else {
    Join-Path $root $ReportDir
}
New-Item -ItemType Directory -Force -Path $reportPath | Out-Null
$stamp = Get-Date -Format "yyyyMMdd-HHmmss"
$report = Join-Path $reportPath "pitr-drill-$stamp.md"
$start = Get-Date

$argsList = @("-NoProfile", "-ExecutionPolicy", "Bypass", "-File", $drill)
if ($Docker) { $argsList += @("-Docker", $Docker) }
if ($Keep) { $argsList += "-Keep" }

$output = & powershell @argsList 2>&1
$code = $LASTEXITCODE
$seconds = [math]::Round(((Get-Date) - $start).TotalSeconds, 1)
$status = if ($code -eq 0) { "PASS" } else { "FAIL" }

$taskLine = "not requested"
if ($RegisterTask) {
    $self = $MyInvocation.MyCommand.Path
    $taskCmd = "powershell -NoProfile -ExecutionPolicy Bypass -File `"$self`" -ReportDir `"$reportPath`""
    & schtasks /Create /SC DAILY /ST $DailyAt /TN $TaskName /TR $taskCmd /F | Out-Null
    $taskLine = if ($LASTEXITCODE -eq 0) { "registered daily at $DailyAt" } else { "registration failed (exit $LASTEXITCODE)" }
}

$body = @(
    "# PITR drill $stamp",
    "",
    "- Status: **$status**",
    "- Exit code: $code",
    "- Duration: ${seconds}s",
    "- Keep containers: $([bool]$Keep)",
    "- Scheduled task: $taskLine",
    "",
    "## Output",
    "",
    '```text',
    ($output | Out-String).TrimEnd(),
    '```',
    ""
) -join [Environment]::NewLine

[System.IO.File]::WriteAllText($report, $body, (New-Object System.Text.UTF8Encoding($false)))
Write-Output "report=$report status=$status exit=$code elapsed=${seconds}s"
exit $code
