# Postgres PITR 演练

> 适用：单机或主备 Postgres 的“恢复到指定时间点”演练。不是自动恢复工具，也不是生产操作脚本。

## 1. 前提

- Postgres 已开启 WAL 归档：`wal_level=replica`、`archive_mode=on`、可用的 `archive_command`。
- 有一份可用的基础备份（`pg_basebackup` 或物理备份）。
- 演练使用**独立数据目录和端口**，不碰生产实例。
- 记录演练前的基准：运行数、最近 `created_at`、关键表行数。

## 2. 参数

| 参数 | 示例 | 说明 |
|------|------|------|
| 基础备份 | `/backup/base` | `pg_basebackup` 输出目录 |
| WAL 归档 | `/backup/wal` | `restore_command` 读取位置 |
| 目标时间 | `2026-11-24 08:00:00+00` | 要恢复到的时刻 |
| 演练数据目录 | `/restore/data` | 新目录，绝不复用生产目录 |
| 演练端口 | `15433` | 与生产错开 |

## 3. 步骤

1. 记录目标时间与当前 LSN：

   ```sql
   SELECT now(), pg_current_wal_lsn();
   ```

2. 准备基础备份（如已有可跳过）：

   ```bash
   pg_basebackup -h <host> -p <port> -U postgres -D /backup/base -Fp -Xs -P -R
   ```

3. 复制到演练目录：

   ```bash
   cp -a /backup/base /restore/data
   chown -R postgres:postgres /restore/data
   ```

4. 写恢复配置（`/restore/data/postgresql.auto.conf`）：

   ```conf
   restore_command = 'cp /backup/wal/%f %p'
   recovery_target_time = '2026-11-24 08:00:00+00'
   recovery_target_action = 'promote'
   ```

5. 创建恢复信号并启动演练实例：

   ```bash
   touch /restore/data/recovery.signal
   pg_ctl -D /restore/data -o "-p 15433" -l /restore/pg.log start
   ```

6. 等待日志出现 `redo done` / `database system is ready to accept connections`，然后校验：

   ```sql
   SELECT pg_is_in_recovery(), pg_last_wal_replay_lsn();
   SELECT count(*), max(created_at) FROM runs;
   SELECT count(*) FROM conversation_messages;
   ```

   与演练前基准对比；差异应在目标时间之后的数据范围内。

7. 记录结果并停止演练实例：

   ```bash
   pg_ctl -D /restore/data stop
   ```

## 4. 记录模板

| 项 | 值 |
|----|----|
| 演练日期 | |
| 目标时间 | |
| 基础备份时间 | |
| WAL 归档范围 | |
| 实际恢复到的 LSN / 时间 | |
| 运行数（演练前 / 恢复后） | |
| 最近 created_at（演练前 / 恢复后） | |
| 关键表行数差异 | |
| RPO（丢失时间窗口） | |
| RTO（从开始到可查询） | |
| 结论 / 问题 | |

## 5. 边界

- 没有 WAL 归档时只能恢复到基础备份时刻，不能做真正的时间点恢复。
- 本流程不自动切换生产流量；生产恢复仍需人工审批与回退方案。
- 容器化部署需要把备份/WAL 目录挂载进临时容器，步骤等价。
- 演练结果必须附真实命令输出，不能只写“通过”。

## 6. Docker 快速演练

`scripts/pitr-drill.ps1` 在隔离容器里跑完整流程：基础备份 → WAL 归档 → 恢复到目标时间 → 校验。只创建 `baize-pitr-*` 容器和卷，默认结束后清理，不会碰现有实例。

```powershell
.\scripts\pitr-drill.ps1 -Docker "<path-to-docker.exe>"
```

`-Docker` 可省略（Docker 在 PATH 时）。可选参数：`-SourcePort`（默认 15433）、`-RestorePort`（默认 15434）、`-Keep`（保留现场）。2026-11-25 已实际执行一次，结果记录在私有 note `eval-artifacts/pitr-drill-20261125/report.md`：RTO 16.1 秒，目标时间之后的写入被正确排除。
