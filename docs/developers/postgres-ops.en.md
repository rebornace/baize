# Postgres multi-replica, backup and PITR

> Applies to `store.driver=postgres` + `middleware.driver=redis`.
> Goal: replicas can start without DDL races, and operators can back up, restore and do point-in-time recovery. Managed databases and automatic failover are out of scope.

---

## 1. Multi-replica prerequisites

```yaml
store:
  driver: postgres
  dsn: postgres://baize:...@db:5432/baize?sslmode=disable

middleware:
  driver: redis
  redis:
    addr: redis:6379
    stream: baize:runs
    consumer_group: baize-workers
    events_channel: baize:run-events
```

- Queue/events use Redis; each replica has a unique consumer name (generated per process by default).
- All replicas share one Postgres; do not run separate SQLite stores.
- Migrations are serialized by a **Postgres advisory lock** (`pg_advisory_lock`, 60s acquire timeout).
- Replicas may start in parallel: one migrates, the rest wait then continue.

## 2. Migrations

- `CREATE TABLE/INDEX IF NOT EXISTS` and `ALTER TABLE ADD COLUMN IF NOT EXISTS`.
- Fixed lock id (`2026112301`); the lock uses a separate connection and is released after migration.
- A replica exits if migration fails; do not send traffic before it is done.
- Back up before upgrades; run large `ALTER`s in a maintenance window.

## 3. Logical backup

```bash
pg_dump -Fc -d "$DSN" -f baize-$(date +%Y%m%d-%H%M).dump
pg_restore --list baize-*.dump | head
createdb -h db -U baize baize_restore
pg_restore -d "postgres://baize@db:5432/baize_restore" baize-*.dump
```

After restore:

1. Start one replica; check `/healthz`, conversations, Run history and approvals.
2. Start the others; confirm the Redis consumer group does not double-claim.
3. Check `GET /metrics` / `GET /v0/settings/logs` for migration and queue errors.

## 4. PITR (base backup + WAL)

Postgres:

```conf
wal_level = replica
archive_mode = on
archive_command = 'test ! -f /archive/%f && cp %p /archive/%f'
```

Base backup:

```bash
pg_basebackup -h db -U repl -D /backup/base -Ft -z -P -X stream
```

Restore:

```bash
mv $PGDATA $PGDATA.old
tar -xzf /backup/base/base.tar.gz -C $PGDATA
touch $PGDATA/recovery.signal
cat >> $PGDATA/postgresql.auto.conf <<'EOF'
restore_command = 'cp /archive/%f %p'
recovery_target_time = '2026-11-23 10:00:00+08'
recovery_target_action = 'promote'
EOF
pg_ctl start -D $PGDATA
```

Acceptance:

- Counts for `runs` / `events` / `messages` / `eval_cases` match the target time.
- Start one replica; run a read query and one HITL resume.
- No duplicate Runs / queue consumption; reset the Redis stream/group if needed.

## 5. Failover

1. Promote the standby (or switch to the new primary).
2. Update `store.dsn` on all replicas; rolling restart.
3. Old leases expire by TTL and the reconciler re-enqueues.
4. Watch `run.error`, `hitl.expired`, `channel.outbound.dead`.

## 6. Monitoring

- `pg_stat_activity`: connections, long transactions, lock waits.
- `pg_locks`: migration/row lock waits.
- Replication lag when using streaming replication.
- Baize: `GET /metrics`, `GET /v0/settings/logs`, `workflow.completed` / `lase.decision` / `hitl.*` events.
- Redis: stream length, consumer-group pending, event-channel disconnects.

## 7. Known multi-replica limits

- HITL expiry/approval is process-local; replicas can race and the last terminal write wins.
- The active-Run guard is a process-local conversation lock; two replicas can still open Runs for one conversation.
- Webhook inbound has a local dedupe TTL; replicas may process twice, so external idempotency is required.
- Schedule claims use a lease; extreme delays can still open twice.
- Pending SSE now has a cross-replica nudge, with a 30s fallback.

These limits do not block single-writer or low-concurrency replicas. Strict approval/one-active-Run guarantees need database-level constraints, designed separately.

## 8. Release/ops checklist

- [ ] Migration lock works (two replicas start together with no DDL conflict)
- [ ] Redis queue/events configured; consumer names unique
- [ ] Logical backup restores
- [ ] PITR recovers to the target time
- [ ] Rolling restart creates no duplicate Runs
- [ ] Dashboards cover connections, locks, queue and HITL
