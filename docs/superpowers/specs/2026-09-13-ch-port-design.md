# CH-PORT：Runtime 可配端口 + 适配器动态端口

- 日期：2026-09-13
- 状态：已交付（2026-09-13）
- 归属：开源首版史诗 **CH-PORT**；确认清单「动态端口」；与 **CH-OUTBOX** 同域串行（PORT 先）
- 承接：`2026-09-07-phase2b-weixin-adapter.md` §14「动态端口发现」后续增强；Runtime YAML `listen` 已有、缺 env 覆盖与 autostart 动态发现
- 产品决策来源：`docs/superpowers/notes/2026-09-13-v1-product-confirmation-checklist.md`
- 实现计划：`docs/superpowers/plans/2026-09-13-ch-port.md`

## 1. 目标 / 非目标

### 1.1 目标

1. **Runtime listen**：环境变量 `BAIZE_LISTEN`（非空）覆盖 YAML `listen`，便于同机部署避开占用端口；改端口仍须**重启进程**。
2. **Autostart webhook 适配器**：默认动态绑定（`-addr=127.0.0.1:0`），经**端口文件**回报实际地址；baize supervisor 据此拼装 `admin_url` / `outbound_url` / `healthz`，多实例不再手改撞端口。
3. **固定端口逃生口**：独立部署或显式固定 `-addr` / URL 的配置继续可用。

### 1.2 非目标

- listen / 适配器端口热切（不进 F-HOT）
- 设置 UI 编辑端口
- 认领通用 `PORT` 环境变量（避免与其他框架冲突）
- **CH-OUTBOX**、非 webhook 渠道、飞书/钉钉真实适配器
- 强制独立部署适配器使用动态端口

## 2. Runtime：`BAIZE_LISTEN`

| 项 | 约定 |
|----|------|
| 时机 | YAML 加载并完成默认值填充之后、bootstrap listen 之前 |
| 规则 | `v := strings.TrimSpace(os.Getenv("BAIZE_LISTEN"))`；`v != ""` 则 `cfg.Listen = v` |
| 空 env | 保持 YAML / 默认 `:8080` |
| 下游 | `localHTTPBase(listen)`、适配器 `-baize` 入站基址随实际 listen 变化 |
| 文档 | README / README.zh-CN / `.env.example` 增加一行说明；注明改端口需重启 |

不改：TLS、`data_dir`、mock-ticket 的 `mock_ticket.listen`（仍仅 YAML；本史诗不扩）。

## 3. 适配器：端口文件协议

面向 **weixin-adapter**（及未来同约定的 autostart 适配器）：

| 项 | 约定 |
|----|------|
| 新 flag | `-port-file=<path>`（可选） |
| 有 `-port-file` | 先 `net.Listen`（支持 host `:0`），将实际监听地址写成**单行** `host:port`（如 `127.0.0.1:54321`），再 `http.Serve` |
| 无 `-port-file` | 与今日一致：固定 `-addr`，默认 `127.0.0.1:8090` |
| 写失败 | 进程退出非零；supervisor 视为 spawn/health 失败 |

文件内容不含 scheme、路径或 JSON；仅 `host:port`。覆盖写入同一路径即可（重启/respawn 覆盖）。

## 4. Supervisor / 渠道装配

### 4.1 动态模式（autostart 默认）

当实例 `adapter_autostart=true` 且判定为**动态端口**时，baize：

1. 注入或覆盖为 `-addr=127.0.0.1:0`
2. 注入 `-port-file=<credsDir>/listen.port`（`credsDir` 解析规则不变）
3. spawn 子进程后，在超时内轮询读取端口文件（合法 `host:port`）
4. 运行时拼装（不要求写入持久 channel config 覆盖 YAML 原文）：
   - `admin_url = http://<addr>`
   - `outbound_url = http://<addr>/outbound`
   - `healthzURL = http://<addr>/healthz`
5. 再挂 admin client，走现有 waitHealthz / adopt-orphan / HMAC 兼容检查

配置里原先写死的 `admin_url` / `outbound_url` 在动态模式下仅作**占位/校验旁路**：装配成功后以端口文件为准。

### 4.2 何时走固定端口（逃生口）

满足任一条则**不**注入 `:0` / `-port-file`，沿用配置中的 URL 与 `adapter_args`：

- `adapter_args` 已含 `-addr=` 且地址端口**不是** `0`（显式固定端口）
- 或文档约定的「独立部署」路径：`adapter_autostart` 为 false（本史诗不改独立部署行为）

若 `adapter_args` 已含 `-addr=...:0` 但未带 `-port-file`，baize 仍应注入 `-port-file`（否则无法发现端口）。

### 4.3 Demo / 文档

- `configs/demo.yaml` 与渠道文档：autostart 示例改为动态端口说明；可保留一小段「固定端口」对照
- 更新 phase2b §14：动态发现由「后续」改为「CH-PORT 已交付」指向本规格（实现合并后）

## 5. 错误处理

| 情况 | 行为 |
|------|------|
| 端口文件超时未出现 / 内容非法 | 终止子进程；supervisor 记错；可按现有退避 respawn |
| `BAIZE_LISTEN` 非法导致 bind 失败 | 进程启动失败（与今日非法 `listen` 相同） |
| 固定模式 healthz 不可达 | 现有 `adapter_unreachable` 等行为不变 |

## 6. 测试 DoD

- `BAIZE_LISTEN` 非空覆盖 YAML；空则不覆盖
- weixin-adapter：`-addr=127.0.0.1:0` + `-port-file` 写出实际端口；无 port-file 行为回归
- supervisor/装配：读端口文件后 `admin`/`outbound`/`healthz` URL 正确
- 显式固定 `-addr=127.0.0.1:8090` 不走动态注入
- 不要求本刀 UI E2E

## 7. 实现触点（计划级索引）

| 区域 | 预期 |
|------|------|
| `internal/config` 或 bootstrap 加载后 | 应用 `BAIZE_LISTEN` |
| `cmd/weixin-adapter` | `-port-file`；Listen 后写文件 |
| `internal/channel/webhook` | autostart 注入、读端口文件、运行时 URL |
| `.env.example`、README、demo YAML、phase2b 注记 | 文档 |
| 账本 / 确认清单 | CH-PORT 挂本规格；交付后改状态 |

## 8. 已锁定产品决策

| 项 | 决策 |
|----|------|
| 适配器发现 | **端口文件**（方案 1） |
| Runtime | **YAML + `BAIZE_LISTEN`**；不认 `PORT`；无设置 UI |
| Autostart 默认 | 动态端口；固定端口为逃生口 |
| 与 OUTBOX | 本史诗不实现 outbox；PORT 完成后开 CH-OUTBOX |
