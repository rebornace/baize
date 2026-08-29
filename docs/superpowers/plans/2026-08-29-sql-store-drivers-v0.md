# SQL 存储驱动（P4）实现计划

> **目标：** 驱动注册表 + postgres 范例；同 DSN 挂 Runtime/会话/身份；设置页换库（强确认）+ 自重启；CI 真 PG。

**规格：** `docs/superpowers/specs/2026-08-29-sql-store-drivers-v0-design.md`

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/store/registry.go` | RegisterDriver / ListDrivers / OpenWithOptions |
| `internal/store/postgres.go` | Postgres 驱动 + DDL |
| `internal/store/sqlite.go` | SQLStore + 方言 q() |
| `internal/dbutil/rebind.go` | ? → $n |
| `internal/conversation/sql_dialect.go` | OpenSQL |
| `internal/identity/sql_dialect.go` | OpenSQL |
| `internal/config/store_overlay.go` | 写 local yaml |
| `internal/bootstrap/reexec.go` | 自重启 |
| `internal/api/server_store_settings.go` | GET/PUT/POST restart |
| `web/chat/src/pages/StorageSettings.tsx` | 设置 UI |
| `.github/workflows/ci.yml` | Postgres service |

---

## 任务清单

- [x] 驱动注册表与 OpenWithOptions
- [x] SQLStore 方言 + postgres 实现
- [x] conversation/identity OpenSQL
- [x] bootstrap 挂 SQL 后端
- [x] 设置 API + 落盘 + 自重启
- [x] 设置页 Storage
- [x] CI Postgres + BAIZE_TEST_PG_DSN
- [x] 单测与文档
