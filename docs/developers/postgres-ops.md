# Postgres 多副本、备份与 PITR

> 适用：`store.driver=postgres` + `middleware.driver=redis`。
> 目标：多副本启动不冲突、可备份/恢复、可做时间点恢复；不承诺托管数据库或自动 failover。

---

## 1. 多副本前置

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

- 队列/事件走 Redis；每个副本用不同 `consumer_name`（默认按进程生成）。
- 所有副本连同一个 Postgres；不要各自用 SQLite。
- 迁移由 **Postgres advisory lock** 串行化（`pg_advisory_lock`，60s 获取超时）。
- 启动顺序可并行；先拿到锁的副本迁移，其他副本等待后继续。

## 2. 迁移

- `CREATE TABLE/INDEX IF NOT EXISTS` + `ALTER TABLE ADD COLUMN IF NOT EXISTS`。
- 迁移锁 ID 固定（`2026112301`）；锁连接与业务连接池分离，迁移结束即释放。
- 迁移失败时该副本退出；不要在迁移未完成时接流量。
- 升级前先备份；大表 `ALTER` 建议在维护窗口执行。

## 3. 逻辑备份

```bash
# 备份
pg_dump -Fc -d "$DSN" -f baize-$(date +%Y%m%d-%H%M).dump

# 校验可读
pg_restore --list baize-*.dump | head

# 恢复到新库（先建空库）
createdb -h db -U baize baize_restore
pg_restore -d "postgres://baize@db:5432/baize_restore" baize-*.dump
```

恢复后：

1. 先只启动一个副本，确认 `/healthz`、会话列表、Run 历史、审批可见。
2. 再启动其他副本；确认 Redis 消费者组没有重复 claim。
3. 用 `GET /metrics` / `GET /v0/settings/logs` 检查迁移与队列错误。

## 4. PITR（基础备份 + WAL）

Postgres 配置：

```conf
wal_level = replica
archive_mode = on
archive_command = 'test ! -f /archive/%f && cp %p /archive/%f'
```

基础备份：

```bash
pg_basebackup -h db -U repl -D /backup/base -Ft -z -P -X stream
```

恢复：

```bash
# 停服务，保留当前数据目录
mv $PGDATA $PGDATA.old
tar -xzf /backup/base/base.tar.gz -C $PGDATA
# 清空 PGDATA 后放回 base 备份内容，创建 recovery.signal
touch $PGDATA/recovery.signal
cat >> $PGDATA/postgresql.auto.conf <<'EOF'
restore_command = 'cp /archive/%f %p'
recovery_target_time = '2026-11-23 10:00:00+08'
recovery_target_action = 'promote'
EOF
pg_ctl start -D $PGDATA
```

验收：

- 恢复到目标时间点后，`runs` / `events` / `messages` / `eval_cases` 计数与预期一致。
- 启动一个副本，跑一次只读查询与一次 HITL 恢复。
- 确认没有重复 Run / 重复队列消费；必要时重置 Redis stream/consumer group。

## 5. Failover / 切换

1. 提升 standby（或切到新主库）。
2. 更新所有副本 `store.dsn`；滚动重启。
3. 检查 Redis 队列：旧主未完成的 lease 会在 TTL 后由 reconciler 重新入队。
4. 观察 `run.error`、`hitl.expired`、`channel.outbound.dead`。

## 6. 监控

- `pg_stat_activity`：连接数、长事务、锁等待。
- `pg_locks`：迁移锁/行锁等待。
- 复制延迟（如使用 streaming replication）。
- Baize：`GET /metrics`、`GET /v0/settings/logs`、`workflow.completed` / `lase.decision` / `hitl.*` 事件。
- Redis：stream 长度、consumer group pending、事件频道断线。

## 7. 已知多副本限制

- HITL 过期与审批仍是进程内串行；多副本可能竞争，最后一个终态写入生效。
- 会话活跃 Run 的 guard 是进程内锁；多副本仍可能同会话开两个 Run。
- Webhook inbound 有本地去重 TTL；多副本可能重复处理，需外部幂等。
- 定时任务用 claim lease，极端超时可能重复打开。
- SSE pending 现在有跨副本 nudge，但仍保留 30s 兜底。

这些限制不阻塞单写多读或低并发多副本；强一致审批/唯一活跃 Run 需要数据库级约束，后续再设计。

## 8. 发布/运维检查清单

- [ ] 迁移锁生效（两副本同时启动无 DDL 冲突）
- [ ] 队列/事件 Redis 配置一致，consumer name 唯一
- [ ] 逻辑备份可恢复
- [ ] PITR 恢复到目标时间点
- [ ] 滚动重启期间无重复 Run
- [ ] 监控看板覆盖连接、锁、队列、HITL
