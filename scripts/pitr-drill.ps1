# Isolated Docker PITR drill for Postgres 16.
# Creates only baize-pitr-* containers/volumes on ports 15433/15434 and
# removes them unless -Keep is passed. Never touches an existing instance.
#
# Usage:
#   .\scripts\pitr-drill.ps1 -Docker "<path-to-docker.exe>"
param(
    [int]$SourcePort = 15433,
    [int]$RestorePort = 15434,
    [string]$Docker = "",
    [switch]$Keep
)

$ErrorActionPreference = "Stop"
$drillStart = Get-Date
$docker = $Docker
if (-not $docker) { $docker = (Get-Command docker -ErrorAction SilentlyContinue).Source }
if (-not $docker) { $docker = $env:DOCKER_BIN }
if (-not $docker) { throw "docker not found; pass -Docker <path> or put it on PATH" }

$tag = "postgres:16"
$src = "baize-pitr-src"
$rst = "baize-pitr-restore"
$srcData = "baize-pitr-src-data"
$rstData = "baize-pitr-restore-data"
$archiveVol = "baize-pitr-archive"
$backupVol = "baize-pitr-backup"
$allVols = @($srcData, $rstData, $archiveVol, $backupVol)

function Run([string[]]$cmd) {
    & $docker @cmd
    if ($LASTEXITCODE -ne 0) { throw "docker failed: $($cmd -join ' ')" }
}

function Sql([string]$container, [string]$sql) {
    $out = & $docker exec $container psql -U postgres -d baize -tAX -c $sql
    if ($LASTEXITCODE -ne 0) { throw "psql failed: $sql" }
    return (($out -join "`n").Trim())
}

function Wait-Ready([string]$container) {
    $old = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    try {
        for ($i = 0; $i -lt 90; $i++) {
            & $docker exec $container pg_isready -U postgres *> $null
            if ($LASTEXITCODE -eq 0) { return }
            Start-Sleep -Seconds 1
        }
    } finally {
        $ErrorActionPreference = $old
    }
    throw "Postgres did not become ready: $container"
}

# Fresh isolated containers and volumes; never touches baize-pg.
$oldPreference = $ErrorActionPreference
$ErrorActionPreference = "SilentlyContinue"
& $docker rm -f $src $rst *> $null
foreach ($v in $allVols) { & $docker volume rm $v *> $null }
$ErrorActionPreference = $oldPreference
foreach ($v in $allVols) { Run @("volume", "create", $v) }
Run @("run", "--rm", "-v", "${archiveVol}:/archive", "-v", "${backupVol}:/backup",
    "--entrypoint", "sh", $tag, "-c", "chmod 777 /archive /backup")

Run @("run", "-d", "--name", $src,
    "-e", "POSTGRES_PASSWORD=postgres", "-e", "POSTGRES_DB=baize",
    "-p", "${SourcePort}:5432",
    "-v", "${srcData}:/var/lib/postgresql/data",
    "-v", "${archiveVol}:/archive", "-v", "${backupVol}:/backup",
    $tag,
    "-c", "wal_level=replica",
    "-c", "archive_mode=on",
    "-c", "archive_command=test ! -f /archive/%f && cp %p /archive/%f",
    "-c", "max_wal_senders=5",
    "-c", "archive_timeout=60")
Wait-Ready $src

Sql $src "CREATE TABLE runs (id text primary key, created_at timestamptz not null default now())" | Out-Null
Sql $src "CREATE TABLE conversation_messages (id bigserial primary key, body text)" | Out-Null

# Base backup before the target rows.
Run @("exec", $src, "pg_basebackup", "-U", "postgres", "-D", "/backup/base", "-Ft", "-z", "-Xs")

Sql $src "INSERT INTO runs (id) VALUES ('run_before')" | Out-Null
Sql $src "INSERT INTO conversation_messages (body) VALUES ('before')" | Out-Null
$target = Sql $src "SELECT clock_timestamp()"
$lsnBefore = Sql $src "SELECT pg_current_wal_lsn()"
Start-Sleep -Seconds 2
Sql $src "INSERT INTO runs (id) VALUES ('run_after')" | Out-Null
Sql $src "INSERT INTO conversation_messages (body) VALUES ('after')" | Out-Null
$lsnAfter = Sql $src "SELECT pg_current_wal_lsn()"

$archivedBefore = [int](Sql $src "SELECT archived_count FROM pg_stat_archiver")
Sql $src "SELECT pg_switch_wal()" | Out-Null
for ($i = 0; $i -lt 60; $i++) {
    $archived = [int](Sql $src "SELECT archived_count FROM pg_stat_archiver")
    if ($archived -gt $archivedBefore) { break }
    Start-Sleep -Seconds 1
}
if ($archived -le $archivedBefore) { throw "WAL was not archived" }
$failed = [int](Sql $src "SELECT failed_count FROM pg_stat_archiver")
if ($failed -ne 0) { throw "WAL archival failed: $failed" }

$restoreScript = @'
set -euo pipefail
rm -rf /restore/* /restore/.[!.]* /restore/..?* 2>/dev/null || true
tar -xzf /backup/base/base.tar.gz -C /restore
rm -rf /restore/pg_wal
mkdir -p /restore/pg_wal
tar -xzf /backup/base/pg_wal.tar.gz -C /restore/pg_wal
chown -R postgres:postgres /restore
{
  echo "restore_command = 'cp /archive/%f %p'"
  echo "recovery_target_time = '$PITR_TARGET'"
  echo "recovery_target_action = 'promote'"
} >> /restore/postgresql.auto.conf
touch /restore/recovery.signal
'@

$restoreB64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($restoreScript))
$restoreCmd = "printf '%s' '$restoreB64' | base64 -d | bash"

Run @("run", "--rm", "--entrypoint", "bash",
    "-e", "PITR_TARGET=$target",
    "-v", "${rstData}:/restore", "-v", "${backupVol}:/backup", "-v", "${archiveVol}:/archive",
    $tag, "-c", $restoreCmd)

Run @("run", "-d", "--name", $rst,
    "-e", "POSTGRES_PASSWORD=postgres", "-e", "PGDATA=/restore",
    "-p", "${RestorePort}:5432",
    "-v", "${rstData}:/restore", "-v", "${backupVol}:/backup", "-v", "${archiveVol}:/archive",
    $tag)

$promoted = $false
$oldPreference = $ErrorActionPreference
$ErrorActionPreference = "SilentlyContinue"
for ($i = 0; $i -lt 120; $i++) {
    $out = & $docker exec $rst psql -U postgres -d baize -tAX -c "SELECT pg_is_in_recovery()" 2>$null
    if ($LASTEXITCODE -eq 0 -and (($out -join "").Trim()) -eq "f") {
        $promoted = $true
        break
    }
    Start-Sleep -Seconds 1
}
$ErrorActionPreference = $oldPreference
if (-not $promoted) {
    & $docker logs --tail 40 $rst
    throw "restore did not promote"
}

$ids = Sql $rst "SELECT coalesce(string_agg(id, ',' ORDER BY id), '') FROM runs"
$messages = Sql $rst "SELECT count(*) FROM conversation_messages"
$replayed = Sql $rst "SELECT pg_last_wal_replay_lsn()"
$inRecovery = Sql $rst "SELECT pg_is_in_recovery()"
$configuredTarget = Sql $rst "SHOW recovery_target_time"
$rto = [math]::Round(((Get-Date) - $drillStart).TotalSeconds, 1)

"target_time=$target"
"configured_target=$configuredTarget"
"lsn_before=$lsnBefore"
"lsn_after=$lsnAfter"
"replayed_lsn=$replayed"
"in_recovery=$inRecovery"
"archived_count=$archived"
"archived_failed=$failed"
"runs=$ids"
"conversation_messages=$messages"
"rto_seconds=$rto"

if ($ids -ne "run_before") { throw "PITR verification failed: runs=$ids" }
if ($messages -ne "1") { throw "PITR verification failed: messages=$messages" }

"PITR_DRILL_OK"

if (-not $Keep) {
    $oldPreference = $ErrorActionPreference
    $ErrorActionPreference = "SilentlyContinue"
    & $docker rm -f $src $rst *> $null
    foreach ($v in $allVols) { & $docker volume rm $v *> $null }
    $ErrorActionPreference = $oldPreference
}
