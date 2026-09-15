**中文** | [English](./deployment.en.md)

# 部署：Runtime 与渠道适配器

Baize 是单个静态 Go 二进制。**消息渠道**通过旁路适配器进程对接；本仓库当前提供的适配器是个人微信（iLink）渠道 **`weixin-adapter`**（`cmd/weixin-adapter`）。白泽与适配器通过 HMAC 签名的 JSON over HTTP 交互：

| 方向 | 路径 |
|------|------|
| 适配器 → baize | `POST /v0/channels/{name}/inbound` |
| baize → 适配器 | `/outbound`（发消息）、`/admin/*`（登录 / 状态 / start / stop / shutdown）、`GET /healthz` |

每个主机只选一种模式：**Autostart** 或 **Standalone**。

## Autostart（桌面 / 演示 / 单机，含 Windows）

baize 拉起并托管适配器子进程：崩溃指数退避重启；关机时优雅结束子进程。`secret` 留空则自动生成并持久化共享 HMAC。`creds.json` 默认在 `adapter_creds_dir`（如 `./data/channels/weixin`）。

先构建适配器（baize 从 `PATH` 或 `./bin/` 查找）：

```bash
go build -o bin/weixin-adapter ./cmd/weixin-adapter   # Windows: bin/weixin-adapter.exe
go run ./cmd/baize demo                               # 或 Windows: .\demo.cmd
```

渠道块示例（与样板 `configs/demo.yaml` 一致：**默认动态端口**，`adapter_args` 勿写固定 `-addr`；`outbound_url` / `admin_url` 为占位，启动后由 `credsDir/listen.port` 覆盖）：

```yaml
channels:
  - name: weixin
    type: webhook
    enabled: true
    config:
      source: weixin
      outbound_url: http://127.0.0.1:8090/outbound
      admin_url: http://127.0.0.1:8090
      assignee: channel:weixin
      supports_vision: "true"
      adapter_autostart: "true"
      adapter_command: weixin-adapter
      adapter_args: "-creds=./data/channels/weixin"
      adapter_creds_dir: ./data/channels/weixin
```

需要固定端口时，在 `adapter_args` 写非 0 的 `-addr`（如 `-addr=127.0.0.1:8090`）。此模式**不要**再启用 `weixin-adapter.service`——子进程归 baize。

## Standalone（生产 Linux / 容器）

适配器独立服务；baize **不**拉起子进程。用 systemd `Restart=always` 或 compose `restart: unless-stopped` 做看门狗。共享密钥必须在两侧显式配置。

### baize 渠道配置

`adapter_autostart: "false"`；不要设 `adapter_command` / `adapter_args`：

```yaml
channels:
  - name: weixin
    type: webhook
    enabled: true
    config:
      source: weixin
      adapter_autostart: "false"
      admin_url: http://<adapter-host>:8090
      outbound_url: http://<adapter-host>:8090/outbound
      assignee: channel:weixin
      supports_vision: "true"
      secret: "<shared-secret>"
      outbound_secret: "<shared-secret>"   # 省略则回退 secret
```

### 适配器 flags

| Flag | 含义 | 默认 |
|------|------|------|
| `-baize` | baize 入站 URL（必填） | — |
| `-secret` | 共享 HMAC（必填） | — |
| `-addr` | 监听（admin / outbound / healthz） | `127.0.0.1:8090` |
| `-creds` | `creds.json` 目录 | `./data/channels/weixin` |
| `-ilink-base` | 覆盖 iLink API（测试） | — |
| `-port-file` | 动态端口写入文件（autostart 发现用） | — |

### Docker Compose

发布镜像含 `/app/baize` 与 `/app/weixin-adapter`。文件 [`docker-compose.weixin.yml`](../../docker-compose.weixin.yml) 起两个服务，并挂载 [`configs/docker-weixin-standalone.yaml`](../../configs/docker-weixin-standalone.yaml)：

```bash
export BAIZE_API_KEY=sk-...
export WEIXIN_ADAPTER_SECRET='<shared-secret>'   # 生产必改强随机；须与渠道 secret 一致
docker compose -f docker-compose.weixin.yml up --build
```

约定：

- 卷 `baize-data` → `/app/data`；`weixin-data` → `/data/channels/weixin`
- baize → `http://weixin-adapter:8090`；适配器入站 → `http://baize:8080/v0/channels/weixin/inbound`
- 样板默认共享密钥仅供试用；生产须同时改 env 与 YAML（或挂载自有配置）

### systemd

单元来自 [`deploy/systemd/`](../../deploy/systemd/)。也可从 [GitHub Releases](https://github.com/rebornace/baize/releases) 下载预编译 `linux` 包，解压到 `/opt/baize` 后跳过下面的 `go build`：

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /opt/baize/baize ./cmd/baize
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /opt/baize/weixin-adapter ./cmd/weixin-adapter
cp deploy/systemd/baize.service deploy/systemd/weixin-adapter.service /etc/systemd/system/
# 编辑 weixin-adapter.service：REPLACE_WITH_SHARED_SECRET 与渠道 secret 一致
systemctl daemon-reload
systemctl enable --now baize.service
systemctl enable --now weixin-adapter.service   # 仅 Standalone
```

- `baize.service`：`baize serve -config /opt/baize/config.yaml`，`Restart=always`
- Autostart 模式只启 `baize.service`

## 共性

- **共享密钥**：Autostart 可自动生成；Standalone 须两侧一致（渠道 `secret` / `outbound_secret` 与适配器 `-secret`）。
- **凭证目录**：持久化 `-creds`，扫码登录按部署一次即可；单账号对应单适配器实例，勿多进程共用同一凭据目录。
- **优雅退出**：baize 处理 SIGINT/SIGTERM；Autostart 下会经 `/admin/shutdown` → SIGTERM → 强杀结束子进程。Standalone 下适配器由各自编排器管理。
