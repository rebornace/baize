# Baize 设计规格：跨平台与 Docker 服务端部署

> 状态：已批准  
> 日期：2026-08-14  
> 前置：Runtime 已为纯 Go（`modernc.org/sqlite`，无 CGO）、`baize start` / `baize serve` 已落地  
> 依据：产品以服务端旁挂部署为主；需体现 Linux / Windows / macOS 同一份源码可跑

---

## 0. 动机

Go 运行时本身已经跨平台，但仓库表现像 Windows 开发机项目：根目录 `start.cmd` 被 README 单独加粗，只有 PowerShell 脚本，没有 POSIX 入口、没有交叉编译说明、没有容器。服务端（多为 Linux）看不出「拷上去就能跑」的优势。

本里程碑把本机三系统和一种服务端部署（Docker Compose 试用 + `docker run` 生产）做成一等公民。不做 CI 矩阵、systemd、多平台 Release。

---

## 1. 目标与成功标准

**目标：** 同一份源码可在 Linux / macOS / Windows 上以相同心智启动；Linux 服务器可用 Docker 拉起试用栈或只跑 Runtime 旁挂真实系统。

**成功标准：**

1. README 中英「快速开始」把三系统写成同一套 `go run` / `go build`，Windows 启动器不再当主入口
2. `./scripts/start.sh` 与 `start.cmd` 对等：未设时写入国内 `GOPROXY`，再 `go run ./cmd/baize start`；找不到 `go` 则非零退出
3. 文档给出 linux / darwin / windows 的 `GOOS`/`GOARCH` `go build` 示例，并写明无 CGO
4. `docker compose up --build` 起 `baize`（8080）与 `mock-ticket`（18080）；UI 与 mock LLM 工单样板可用
5. 生产路径：`docker run` 挂自己的 yaml（`mock_ticket` 关闭、`base_url` 指向真实系统）和 `data` 卷，不强制拉起 mock-ticket
6. `configs/default.yaml` 与本机 `baize start` 行为不变

---

## 2. 范围

### 做

| 层 | 内容 |
|----|------|
| 文档 | README 中英：并列源码启动、本机二进制、Docker；交叉编译示例 |
| POSIX 启动 | `scripts/start.sh`（对齐 `scripts/start.ps1` 的 GOPROXY / 缺 go 报错） |
| mock-ticket 进程 | `examples/mock-ticket/cmd/mock-ticket/main.go`，默认 `:18080` |
| 镜像 | 多阶段 Dockerfile：`CGO_ENABLED=0`，镜像内 `/app/baize`、`/app/mock-ticket`、OpenAPI 与 compose 配置 |
| Compose | `docker-compose.yml`：两服务；baize **不** `depends_on` mock-ticket |
| 容器配置 | `configs/docker.yaml`：试用专用，不替代 `default.yaml` |
| 忽略 | `.dockerignore`：排除 `data/`、`.env`、`.git`、`web/chat/node_modules` |

### 不做

- CI 多系统矩阵、GitHub Actions
- systemd unit、goreleaser、向镜像仓库发布
- 把 `export-public.ps1` 改成 bash
- distroless、buildx 多架构
- 改开箱默认 Connector；`baize start` 不调用 Docker

---

## 3. 架构

```
本机（任意 OS）          试用（Compose）              生产（docker run）
go run / start.sh      mock-ticket :18080           baize serve + 自备 yaml
start.cmd / start.ps1         │                     mock_ticket: off
        │                     ▼                     base_url → 真实系统
        ▼               baize serve                 data 卷持久化
  baize start           docker.yaml
  （进程内 mock-ticket）  base_url=http://mock-ticket:18080
```

一份镜像、两个二进制入口。试用用 compose 叠 mock-ticket；生产只用 `baize` 镜像 + 挂载配置。

---

## 4. 本机启动与文档

### 4.1 POSIX `scripts/start.sh`

- 切到仓库根目录（脚本位于 `scripts/`）
- 若 `GOPROXY` 未设：`https://goproxy.cn,direct`；若 `GOSUMDB` 未设：`sum.golang.google.cn`（与 `start.ps1` 一致）
- `command -v go` 失败 → stderr 提示安装 Go 1.22+，退出 1
- `exec go run ./cmd/baize start "$@"`
- 可执行位：`chmod +x` 后提交（git 文件模式）

不在脚本里写死 `%USERPROFILE%\sdk\go`（那是本机 Windows SDK 路径）；Linux/macOS 依赖 PATH 上的 `go`。

### 4.2 README

「快速开始」顺序：

1. 源码：`go run ./cmd/baize start`；可选 `./scripts/start.sh` 或 `.\start.cmd`
2. 本机二进制：`GOOS=linux GOARCH=amd64 go build -o baize ./cmd/baize`（darwin/amd64、windows/amd64 各一行）；注明 `CGO_ENABLED=0` 且默认 SQLite 无需 C 编译器
3. Docker：链到同页「服务端（Docker）」

Windows 段落降为可选一行，不再「**Windows 一键启动**」作为主路径。

### 4.3 交叉编译

只文档化，不新增 Makefile / goreleaser。输出名：Unix `baize`，Windows `baize.exe`。

---

## 5. mock-ticket 独立进程

`examples/mock-ticket/cmd/mock-ticket/main.go`：

- `package main`，调用现有 `mockticket.NewHandler()`
- 默认 listen `:18080`；环境变量 `BAIZE_MOCK_TICKET_LISTEN` 可覆盖（与 http-plugin 的 `BAIZE_HTTP_PLUGIN_LISTEN` 同模式）
- `baize start` 仍用 `bootstrap` 进程内 `ListenAndServe`，不改这条路径

---

## 6. Docker

### 6.1 Dockerfile

- 构建阶段：官方 `golang:1.22`（或 patch 兼容的 1.22.x），`ARG GOPROXY=https://goproxy.cn,direct`
- `CGO_ENABLED=0` 构建 `./cmd/baize` 与 `./examples/mock-ticket/cmd/mock-ticket`
- 运行阶段：Alpine；`WORKDIR /app`
- 复制：`baize`、`mock-ticket`、`configs/docker.yaml`、`examples/mock-ticket/openapi.yaml`（保持相对路径，使 `connector.spec` 仍为 `examples/mock-ticket/openapi.yaml`）
- 默认 `CMD`：`["/app/baize", "serve", "-config", "/app/configs/docker.yaml"]`（单容器生产默认不带 mock-ticket）
- 暴露 8080（文档说明 mock-ticket 需覆盖 command 才听 18080）

### 6.2 `configs/docker.yaml`

从 `default.yaml` 拷语义，仅改：

- `store.sqlite_path: /app/data/baize.db`
- `mock_ticket.listen: "off"`
- `connector.base_url: http://mock-ticket:18080`

其余（agent、HITL 名单、mock LLM、spec 路径）与开箱样板一致。不把密钥写进该文件。

### 6.3 `docker-compose.yml`

```yaml
services:
  mock-ticket:
    build: .
    command: ["/app/mock-ticket"]
    ports: ["18080:18080"]
  baize:
    build: .
    ports: ["8080:8080"]
    volumes:
      - baize-data:/app/data
    # 故意不 depends_on mock-ticket
volumes:
  baize-data:
```

baize 用镜像默认 CMD（`serve` + `docker.yaml`）。

试用：`docker compose up --build`（两服务都起）。  
生产：`docker run -p 8080:8080 -v $PWD/my.yaml:/app/configs/docker.yaml -v baize-data:/app/data <image>`，yaml 中 `mock_ticket.listen: off` 且 `base_url` 为真实系统。不要用这份 compose 当生产编排。

### 6.4 `.dockerignore`

至少排除：`.git`、`data/`、`.env`、`configs/default.local.yaml`、`web/chat/node_modules`、`*.exe`。

---

## 7. 错误处理

| 场景 | 期望 |
|------|------|
| `start.sh` 无 `go` | 退出 1，提示安装 Go 1.22+ |
| compose 只起 baize、仍用 `docker.yaml` | 注册 `base_url` 失败（与本机 mock-ticket 未开相同）；README 写明试用要两服务一起 `up` |
| 生产 yaml 的 `base_url` 不可达 | 现有 bootstrap/注册错误，不新增 Docker 专用错误码 |
| 构建 GOPROXY 失败 | 可通过 `--build-arg GOPROXY=...` 覆盖 |

---

## 8. 测试计划

| 场景 | 断言 |
|------|------|
| `go test ./...` | 现有套件全绿 |
| `go build` mock-ticket cmd 与 baize | 本机编译通过 |
| `start.sh` 语法 | `sh -n scripts/start.sh`（有 POSIX sh 的环境） |
| Docker | **不**进 `go test`。有 Docker 时手工：`docker compose config`、`docker compose up --build` 后 `GET /healthz` 与 mock-ticket `/healthz` 为 200 |
| 回归 | `configs/default.yaml` 仍 `type: openapi` + mock-ticket listen `:18080` |

---

## 9. 文档落点

- README / README.zh-CN：快速开始三路径；新节「服务端（Docker）」含 compose 试用与 `docker run` 生产示例
- 不把 `export-public.ps1` 写进主路径
- 架构草案若提到部署，可加一句「Runtime 为无 CGO 单进程，可用 Docker 旁挂」；没有现成部署章则只改 README

---

## 10. 非目标回顾

之后仓库应能回答：「Linux 服务器怎么跑白泽？Windows 开发机呢？」  
不承诺 CI 绿矩阵、systemd、或官方镜像仓库。HTTP 插件参考侧车不进默认 compose（仍按文档手动 `go run`）。

---

*本文档经头脑风暴分节批准后落盘；实现前若路径名有微调，以本文语义为准并更新本文。*
