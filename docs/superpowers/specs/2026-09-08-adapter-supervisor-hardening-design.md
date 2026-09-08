# 适配器进程托管硬化 + 生产部署工件（设计）

> 状态：设计稿（仅私有仓 docs/superpowers，不导出公开仓）
> 日期：2026-09-08
> 范围：方案 A —— webhook 适配器 supervisor 的崩溃看门狗 + 优雅关停，外加生产部署工件（Docker/systemd/部署文档）。

## 1. 背景与问题

阶段二（2A/2B）后，webhook 渠道的 autostart 实例由 baize 进程内的
`internal/channel/webhook/supervisor.go`（约 200 行，纯标准库 `os/exec` +
`net/http`）拉起并托管适配器子进程。它已具备：跨平台 exec、healthz 就绪等待、
孤儿进程收养/拒绝（healthz + HMAC 签名兼容判定）、owned-kill / adopted-shutdown、
进程级 start/stop/restart（`process.go`，暴露到 Web UI）。

实战与代码盘点暴露两个 autostart 形态的短板，以及生产部署落地缺口：

1. **无崩溃看门狗**：`start()` 拉起子进程后没有 goroutine 盯它。适配器自行崩溃
   不会被自动重启；`Status()` 只能探活到 down 报 `start_failed`，需人工在 UI 重启
   或重启 baize。
2. **关停不优雅**：`kill()` 直接 `Process.Kill()`（POSIX=SIGKILL / Win=
   TerminateProcess）。但适配器（`cmd/weixin-adapter/main.go`）**已实现**优雅关停
   （监听 `SIGTERM`/中断 + HMAC `/admin/shutdown`，都会 `stopPolling()` +
   `srv.Shutdown(5s)`），当前强杀没给它收尾机会（可能打断媒体上传/凭据写入）。
3. **生产部署工件不全**：`Dockerfile` 只构建 baize + mock-ticket，未构建
   weixin-adapter；`docker-compose.yml` 仅 baize 单服务；无 systemd 单元示例；
   没有文档说明"autostart 托管"与"适配器独立部署"两种生产形态的取舍。

**非目标（YAGNI）**：不做日志轮转、资源限制（cgroup/ulimit）、崩溃 dump 收集、
崩溃事件聚合 API——这些在容器/systemd 下由外部编排免费提供，归部署侧；不引入
supervisord/PM2/NSSM 等外部进程管理器；不改 webhook 协议契约。

## 2. 决策（已与用户确认）

- **D1 看门狗无限重启**：意外崩溃后按指数退避（1s→2s→4s…封顶 30s）无限重启，
  对齐 systemd `Restart=always` 语义；不设最大重启次数放弃（适配器多为临时故障，
  且 `Status()` 会持续暴露 `restarting`）。手动停止 / 重启 / baize 关停属有意停止，
  不触发重启。
- **D2 优雅关停主路径走 HMAC `/admin/shutdown`**：跨平台统一（Windows 无
  SIGTERM）；POSIX 上 SIGTERM 作为无回调时的兜底。最后手段永远是 `Process.Kill()`
  强杀，保证 baize 关停不被卡死。grace period 5s，对齐适配器自身关停超时。
- **D3 部署工件用独立文件**：新增 `docker-compose.weixin.yml` 而非改默认
  `docker-compose.yml`，避免 `docker compose up` 默认行为变化；systemd 单元放
  `deploy/systemd/`；部署文档进公开仓 `docs/`。

## 3. 崩溃看门狗（自动重启）

`supervisor` 新增状态（`mu sync.Mutex` 保护，与 Channel 的 `procMu`/`settingsMu`
不重叠）：

```go
wantRunning   bool                 // 期望态：true=应在运行
stopCh        chan struct{}        // 关闭即通知 watcher/退避循环退出
restarts      int                  // 累计自动重启次数（诊断/日志，只增）
failStreak    int                  // 连续崩溃/起不来次数，驱动退避；成功 spawn 后归零
backingOff    bool                 // 正处于崩溃后退避等待/重 spawn 循环（restarting 状态源）
lastExit      string               // 最近一次 Wait() 退出描述（诊断）
backoff       func(failStreak int) time.Duration // 可注入；默认指数封顶 30s
grace         time.Duration        // 可注入；默认 5s
gracefulShutdown func(ctx context.Context) error // 注入：c.admin.Shutdown
```

**start(ctx)**：解析/收养逻辑不变。spawn 成功后：
- 置 `wantRunning=true`；新建 `stopCh`；
- `go s.watch(cmd)`。

**watch(cmd)**：
1. `err := cmd.Wait()`；记录 `lastExit`。
2. 加锁：若 `!wantRunning` → 有意停止，置 `s.cmd=nil`、关闭 `watchDone`、返回。
3. 否则为意外崩溃：`restarts++`、`failStreak++`、置 `backingOff=true`，记日志
   （含次数、退出原因）；进入退避重启循环：
   - `delay := s.backoff(failStreak)`（1s,2s,4s…min(30s)）；
   - `select { case <-stopCh: 退出; case <-ctx.Done(): 退出; case <-time.After(delay): }`；
   - 重新 spawn（复用现有 spawn+healthz 等待逻辑，抽成 `spawn(ctx) (*exec.Cmd,error)`）；
     成功 → `failStreak=0`、`backingOff=false`、`go watch(newCmd)`、返回；
     失败（healthz 超时/起不来）→ kill 清理，`failStreak++`、保持 `backingOff=true`，
     继续下一轮退避。
   - 退出循环（stopCh/ctx）时复位 `backingOff=false`。
4. ctx 来源：supervisor 持有一个在 `Channel.Bootstrap` 创建、`Stop()` 时 cancel 的
   `context.Context`（生命周期与 baize 一致），退避循环选中它，关停时 watcher 干净退出。

**有意停止路径**（`stop()`/重启/baize 关停）：先加锁置 `wantRunning=false`、
`close(stopCh)`，再执行 terminate；这样 watcher 在 `Wait()` 返回后看到 false 直接退出，
不会与手动重启竞争 spawn。`process.go` 的重启本就持有 `c.procMu` 串行做
stop→start：旧 watcher 因 `wantRunning=false` 退出，新 `start()` 置 true 并起新
watcher——保证任意时刻最多一个 watcher、不会重复 spawn。

**收养的孤儿不看门**：adopted 进程 `s.cmd == nil`（无 PID 句柄，无法 `Wait()`）。
它若死亡不自动重启（下次手动重启或 baize 重启会重新 spawn）；`Status()` 照常探活
反映 down。文档明确此边界。

**状态上报**：退避重启期间 `Channel.Status()` 返回新 reason `"restarting"`
（`channel.AdapterStatus.Reason` 已是自由字符串，无需改结构）。判定：enabled 且
非 manualStop 且 supervisor 报告"正在退避重启中"——新增 `s.restarting() bool`，
在 `mu` 下返回 `backingOff`（显式标志，不靠推断进程健康）。UI 可显示"适配器重启中"，
区别于开机即失败的 `start_failed`。不新增 API 字段。

## 4. 优雅关停（先礼后兵）

新增统一 `terminate(ctx context.Context) error`，替换现有 `kill()` 的直接强杀
（`kill()` 保留为最后手段的内部实现）：

1. **HMAC 优雅关停（跨平台主路径）**：若 `s.gracefulShutdown != nil`，用带 grace
   超时的 ctx 调 `s.gracefulShutdown(ctx)`（即 `c.admin.Shutdown` → 适配器
   `/admin/shutdown`）。随后 `waitUntilDown(ctx, grace)` 等进程退出；退出即完成。
2. **POSIX 信号兜底**：无 `gracefulShutdown` 回调且 `runtime.GOOS != "windows"`
   时，先 `cmd.Process.Signal(syscall.SIGTERM)`，等待 grace（`waitUntilDown`），
   超时再强杀。
3. **Windows**：无 SIGTERM；依赖第 1 步 `/admin/shutdown`；无回调则直接强杀。
4. **最后手段**：以上未在 grace 内退出 → `cmd.Process.Kill()` + `cmd.Wait()`
   回收（TerminateProcess / SIGKILL），保证 baize 关停不被卡死。

`gracefulShutdown` 回调在 `Channel.Bootstrap` 构造 supervisor 时注入（与现有
`compatible` 回调对称）：`sup.gracefulShutdown = func(ctx) error { return c.admin.Shutdown(ctx) }`，
仅 `c.admin != nil` 时设置。`/admin/shutdown` 是 webhook 适配器管理面契约端点
（非微信专属），任意语言适配器实现即可被优雅关停。

调用点改造：
- `Channel.Stop(ctx)`（baize 关停）：`sup.stop` 内部改走 `terminate`，grace 受
  关停 ctx deadline 约束。
- `process.go` 的 `StopProcess`/`RestartProcess`：owned 进程从 `sup.stop`（强杀）
  改为 `sup.terminate`；adopted 孤儿仍走 `admin.Shutdown` + `waitUntilDown`（现状
  已正确，统一到同一语义）。

## 5. 生产部署工件

**Dockerfile**：build 阶段补 `go build -o /out/weixin-adapter ./cmd/weixin-adapter`，
运行阶段 `COPY --from=build /out/weixin-adapter /app/weixin-adapter`。同一镜像既可
跑 baize（默认 CMD）也可跑适配器（覆盖 command），支撑"容器内 autostart"与
"独立适配器服务"两种形态。

**docker-compose.weixin.yml（新增）**：两个服务——
- `baize`：用同一镜像，挂载 `baize-data`，配 `BAIZE_API_KEY` 等；
- `weixin-adapter`：`command: ["/app/weixin-adapter","-baize=http://baize:8080/...","-secret=${WEIXIN_ADAPTER_SECRET}","-addr=:8090","-creds=/data/channels/weixin"]`，
  `depends_on: [baize]`，共享 secret 环境变量，独立 volume 存凭据。
  这是"独立部署（E 模式）"范本：baize 配置该 webhook 实例 `autostart=false`、
  `admin_url` 指向适配器服务。

**deploy/systemd/（新增）**：
- `baize.service`：baize 主进程单元；
- `weixin-adapter.service`：适配器独立单元，带 `Restart=always`（外部看门狗）、
  `After=network.target`。注释说明：autostart 模式下不需要此单元（baize 自己托管）。

**部署文档（公开仓 docs/deployment.md）**：讲清两种模式取舍——
- **Autostart（桌面/demo/单机，含 Windows）**：baize 托管适配器子进程，零额外进程；
  现具备崩溃看门狗（退避自动重启）+ 优雅关停。双击 demo / 单机部署走这条。
- **独立部署（生产/容器/Linux 服务器）**：适配器作为独立服务（systemd
  `Restart=always` 或容器），baize 经 HTTP 连接、`autostart=false`；进程看门、日志、
  资源限制、重启策略交给 systemd/容器（明确不做日志轮转/cgroup——YAGNI）。
- 给出 docker compose 与 systemd 两种起步命令，以及 secret/凭据目录的挂载约定。

## 6. 测试策略（TDD）

沿用 `supervisor_test.go` 现有手法：`go build` 一个临时测试 exe 到临时目录，
supervisor 直接 exec 它。退避/grace 通过可注入字段缩短（毫秒级），避免测试等待。

**看门狗**：
1. 子进程启动后异常退出（测试 exe 收到信号/运行即退出）且 `wantRunning=true`
   → watcher 自动重启：healthz 恢复 200、`restarts` 增长、最终 running。
2. 有意 `stop()` → `wantRunning=false`，子进程退出后**不**重启（无新进程、healthz
   最终 down）。
3. `restart` 路径（process 层）→ 旧进程被杀、新进程起、无重复 spawn（端口不冲突）。
4. 退避中 cancel ctx（模拟 baize 关停）→ watcher 退出、不再 spawn。
5. adopted 孤儿（`s.cmd==nil`）死亡 → 不自动重启。

**优雅关停**：
6. 测试 exe 收到 SIGTERM（POSIX）写标记文件后退出 0 → `terminate` 在 grace 内
   返回、标记文件存在（证明走了优雅路径而非强杀）。
7. 提供 `gracefulShutdown` 回调（测试 exe 暴露 `/admin/shutdown` 写标记并退出）
   → terminate 走 HMAC 路径、标记存在、进程退出（Windows 语义同此路径）。
8. 忽略优雅信号/关停端点的 exe → 超 grace 后被强杀（短 grace 注入），terminate 返回。

**回归**：现有 `supervisor_test.go`（healthz 超时、收养、拒绝 stale）、
`process_test.go`（start/stop/restart、manualStop、resetForRespawn）全绿；
`settings_test.go` 的 fakeAdmin 补 `restarting` reason 相关断言。

**工件**：`go build ./...` 两个二进制；有 docker 环境时
`docker compose -f docker-compose.weixin.yml config` 校验 YAML，无 docker 则静态
审查；systemd 单元为静态文档、评审为准；部署文档链接自 README。

## 7. 风险与兼容性

- **watcher 与手动重启竞争 spawn**：靠 `wantRunning` + `stopCh` + `procMu` 串行化
  保证任意时刻最多一个 watcher；重启路径先置 false 再 terminate 再 start。测试 3 覆盖。
- **无限重启风暴**：退避封顶 30s，且 healthz 失败也走退避（不会紧循环）；持续失败
  时 `Status()` 报 `restarting`，日志带累计次数，运维可见。不引入"放弃"状态（D1）。
- **优雅关停卡死 baize 退出**：grace 5s 硬上限 + 最终 `Kill()`，且受关停 ctx
  deadline 约束；最坏情况退化为现状的强杀，不会比今天更差。
- **Windows 无 SIGTERM**：主路径本就是 HMAC `/admin/shutdown`（跨平台）；无回调时
  Windows 直接 TerminateProcess（与现状一致）。
- **adopted 孤儿无看门狗**：明确边界，文档说明；独立部署形态由外部 systemd/容器
  负责看门，autostart 形态的孤儿只是 baize 崩溃后的过渡态。
- **公开仓**：本规格在 `docs/superpowers`（不导出）；`docs/deployment.md`、
  Dockerfile、compose、systemd 单元导出到公开仓。

## 8. 涉及文件

- 修改：`internal/channel/webhook/supervisor.go`（watch/backoff/terminate/spawn 抽取、
  新字段、`restarting()`）
- 修改：`internal/channel/webhook/channel.go`（Bootstrap 注入 gracefulShutdown、
  关停 ctx、构造 supervisor 新字段）
- 修改：`internal/channel/webhook/process.go`（stop/restart 走 terminate；
  关停 ctx 传递）
- 修改：`internal/channel/webhook/settings.go`（`Status()` 新增 `restarting` reason）
- 修改/新增测试：`supervisor_test.go`、`process_test.go`、`settings_test.go`
- 修改：`Dockerfile`（构建+拷贝 weixin-adapter）
- 新增：`docker-compose.weixin.yml`、`deploy/systemd/baize.service`、
  `deploy/systemd/weixin-adapter.service`
- 新增：`docs/deployment.md`（公开；README 加入口）


