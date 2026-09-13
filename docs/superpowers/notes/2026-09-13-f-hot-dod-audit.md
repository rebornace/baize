# F-HOT Store 热切 DoD 核验报告

> 日期：2026-09-13  
> 范围：母规格 `2026-09-13-f-production-hardening-design.md` §3（F-HOT）、§4.2 F-HOT 行  
> 计划：`plans/2026-09-13-f-store-hot-swap.md`  
> 结论：**F-HOT 实现阶段可标已交付**；史诗 **F**（C1+KV+HOT）可标整包完成。

## 对照清单

| 项 | 结果 |
|----|------|
| memory↔sqlite 热切后读写落新库 | ✅ `TestHotSwapMemoryToSQLite` |
| 热切失败旧库仍服务 | ✅ `TestHotSwapOpenFailureKeepsStore` |
| overlay 落盘；失败标明 overlay_saved | ✅ `TestPutStoreSettingsHotSwapFailureMarksOverlaySaved` |
| 默认 PUT 热切；restart 逃生舱 | ✅ API + UI（主 CTA 热切 / 次要重启） |
| POST `/v0/settings/reload` | ✅ `TestPostSettingsReload`；ACL Admin |
| SIGHUP（POSIX） | ✅ `sighup_unix.go`；Windows 空实现 + HTTP reload |
| GET `store_config_mismatch` / `effective_driver` | ✅ handler 字段 |
| F-KV：热切后 MigrateStore | ✅ `HotSwap` 内调用 |
| blob/S3 仍重启 | ✅ 本刀未热切 blob |

## 账本动作

- ledger / 确认清单：F 标已交付（含 F-HOT）
- 母规格 §5.1：F-HOT 计划行改为已交付
