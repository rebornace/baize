# Postgres PITR drill

> For a point-in-time recovery drill on a single-node or primary/standby Postgres. This is a runbook, not an automated restore tool or a production script.

## 1. Prerequisites

- WAL archiving is enabled: `wal_level=replica`, `archive_mode=on`, and a working `archive_command`.
- A usable base backup (`pg_basebackup` or physical backup).
- The drill uses a **separate data directory and port**; never touch the production instance.
- Record a baseline first: run count, latest `created_at`, key table row counts.

## 2. Parameters

| Parameter | Example | Meaning |
|-----------|---------|---------|
| Base backup | `/backup/base` | `pg_basebackup` output |
| WAL archive | `/backup/wal` | Source for `restore_command` |
| Target time | `2026-11-24 08:00:00+00` | Point to recover to |
| Drill data dir | `/restore/data` | New directory; never reuse production |
| Drill port | `15433` | Different from production |

## 3. Steps

1. Record the target time and current LSN:

   ```sql
   SELECT now(), pg_current_wal_lsn();
   ```

2. Prepare a base backup (skip if one exists):

   ```bash
   pg_basebackup -h <host> -p <port> -U postgres -D /backup/base -Fp -Xs -P -R
   ```

3. Copy it to the drill directory:

   ```bash
   cp -a /backup/base /restore/data
   chown -R postgres:postgres /restore/data
   ```

4. Configure recovery in `/restore/data/postgresql.auto.conf`:

   ```conf
   restore_command = 'cp /backup/wal/%f %p'
   recovery_target_time = '2026-11-24 08:00:00+00'
   recovery_target_action = 'promote'
   ```

5. Create the recovery signal and start the drill instance:

   ```bash
   touch /restore/data/recovery.signal
   pg_ctl -D /restore/data -o "-p 15433" -l /restore/pg.log start
   ```

6. Wait for `redo done` / `database system is ready to accept connections`, then validate:

   ```sql
   SELECT pg_is_in_recovery(), pg_last_wal_replay_lsn();
   SELECT count(*), max(created_at) FROM runs;
   SELECT count(*) FROM conversation_messages;
   ```

7. Record the result and stop the drill instance:

   ```bash
   pg_ctl -D /restore/data stop
   ```

## 4. Record template

| Item | Value |
|------|-------|
| Drill date | |
| Target time | |
| Base backup time | |
| WAL range | |
| Recovered LSN / time | |
| Run count (before / after) | |
| Latest created_at (before / after) | |
| Key table row deltas | |
| RPO (lost window) | |
| RTO (start to queryable) | |
| Result / issues | |

## 5. Boundaries

- Without WAL archiving you can only restore the base backup; true PITR is impossible.
- This flow does not switch production traffic; production recovery still needs approval and a rollback plan.
- Container deployments need the backup/WAL directories mounted into a scratch container; the steps are equivalent.
- A drill record must include real command output, not just “passed”.
