# 渠道 / supervisor DoD 核验报告

> 日期：2026-09-13  
> 范围：2A webhook 渠道、2B 微信适配器、adapter supervisor 硬化  
> 方法：规格验收项 × 树内证据 × 本机测试命令（有退出码）  
> 结论：**主路径已交付，无阻塞缺口**；少量「清单项可加强单测」记为非阻塞建议。

---

## 0. 环境与命令证据

| 命令 | 结果 |
|------|------|
| `go test ./internal/channel/webhook/ -count=1` | **ok**（~32s） |
| `go test ./cmd/weixin-adapter/... -count=1` | **ok**（adapter + weixinlink） |
| `go test ./tests/integration/ -run 'Webhook\|Channel\|Weixin' -count=1` | **ok**（含 `TestWeixinAdapterE2E`） |
| `go vet ./internal/channel/webhook/... ./cmd/weixin-adapter/...` | **exit 0** |
| `docker compose -f docker-compose.weixin.yml config` | **YAML 合法** |

Go：`C:\Users\Administrator\go-sdk\go\bin\go.exe`（1.25.0）。未跑全量 `go test ./...`（本刀范围限定渠道相关包）。

---

## 1. 结构验收（文件 / 配置）

| 验收意图 | 证据 | 判定 |
|----------|------|------|
| 核心无进程内 weixin | `internal/channel/weixin`、`server_channel_weixin.go` **不存在** | ✅ |
| 适配器同仓 | `cmd/weixin-adapter` 存在；媒体 AES/上传单测齐全 | ✅ |
| 通用 webhook | `internal/channel/webhook/*` + `examples/im-adapter` + `tests/integration/webhook_channel_test.go` | ✅ |
| 默认配置 webhook-weixin + autostart | `configs/minimal.yaml` / `demo.yaml` 显式 `type: webhook` + `adapter_autostart` | ✅ |
| supervisor 工件 | `docker-compose.weixin.yml`、`deploy/systemd/*`、`docs/deployment.md`；Dockerfile 构建 weixin-adapter；README 链部署文档 | ✅ |
| 看门狗 / 优雅关停 | `supervisor_test.go`：`TestWatchdog*`、`TestTerminate*` 等 | ✅ |

---

## 2. 2B §8 回归清单对照（摘要）

| # | 项 | 主要证据 | 判定 |
|---|-----|----------|------|
| 1 | 扫码登录 | `admin_test.go` LoginStart/Status；integration E2E login | ✅ |
| 2 | 登录态回显 | E2E `login_required` → success；`settings` Status 合并 | ✅ |
| 3 | 登出 | admin logout 测试 + E2E | ✅ |
| 4 | 启停 | process start/stop 测试；admin start/stop | ✅ |
| 5 | 白名单 | `inbound_test.go` AllowlistBlocksOutsider | ✅ |
| 6 | 群聊丢弃 | `poller.go` 丢弃 `@chatroom` | ✅ **产品明确不要群聊**；丢弃即正确行为（非待做能力）。可选补单测仅作回归锁，非产品缺口 |
| 7 | 出站前缀 | E2E 断言含 `【助手】` | ✅ |
| 8 | HITL | 通用 `channel/hitl_test` + 引擎/集成 HITL；非微信专用 e2e | ✅（能力在核心，渠道复用） |
| 9 | UI 镜像 | 出站 kind/operator 路径在 webhook 出站测试 | ✅ 偏单元 |
| 10 | context_token | outbound/ilink 相关测试 | ✅ |
| 11–12 | 入站解密 / 出站真发 | `media_test.go`、`upload_test.go`、`TestOutboundImageAndFile` | ✅ |
| 13–14 | convID / 动态 account | `TestDynamicAccountInboundThenOutbound` | ✅ |
| 15 | 管理面鉴权 | `server_channel_managed_test.go` operator 403 | ✅ |
| 16 | autostart 默认部署 | supervisor 拉起测试 + minimal/demo 配置 + deployment.md | ✅（真机扫码手验规格已声明不做 CI） |

---

## 3. supervisor 硬化 §6 对照

看门狗重启 / 有意停止不重启 / lifecycle cancel / HMAC 关停 / force kill / Status `restarting`：均有对应 `TestWatchdog*` / `TestTerminate*` / `TestStatusReportsRestarting*`。**通过。**

规格 2B §9 曾写「v1 不做自动重启」——已被后续 **supervisor 硬化规格** 取代；以硬化规格 + 代码为准。

---

## 4. 结论与仍欠

**结论：渠道 2A/2B + supervisor 硬化可维持「已交付」；本核验未发现阻塞缺口。**

非阻塞建议（不强制本刀改代码）：

1. ~~为 `@chatroom` 丢弃补单测~~ — **产品确认微信渠道不要群聊**；丢弃群消息是既定行为，非缺口。补测仅为可选回归锁。  
2. 可选：全量 `go test ./...` 在 CI/本机再跑一轮作回归安心。

**不做本刀范围：** F 硬化、公开架构 P1/P2、运行参数人话化、真机微信手验。

---

## 5. 账本动作

- ledger §2「渠道/supervisor DoD 核验」标完成，链到本报告。  
- 远程分支 `cursor/sql-store-drivers-p4-7b3c` 已删（见同日 chore）。
