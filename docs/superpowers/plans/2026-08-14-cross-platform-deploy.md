# 跨平台与 Docker 服务端部署 实现计划

> **面向 AI 代理的工作者：** 必需子技能：使用 superpowers:subagent-driven-development（推荐）或 superpowers:executing-plans 逐任务实现此计划。步骤使用复选框（`- [ ]`）语法来跟踪进度。

**目标：** Linux / macOS / Windows 用同一套心智启动 Runtime；Linux 服务器可用 `docker compose` 试用、用 `docker run` 旁挂真实系统。

**架构：** POSIX `scripts/start.sh` 与现有 `start.cmd` 对等。mock-ticket 增加独立 `cmd`。一份多阶段镜像含 `baize` 与 `mock-ticket`；compose 两服务；`configs/docker.yaml` 专供容器试用。不改 `configs/default.yaml`。

**技术栈：** Go 1.22、`CGO_ENABLED=0`、Alpine 镜像、Docker Compose。

**规格：** `docs/superpowers/specs/2026-08-14-cross-platform-deploy-design.md`

**全局约束：**
- 不做 CI 矩阵、systemd、goreleaser、官方镜像仓库、distroless、buildx
- 不改 `export-public.ps1` 为 bash；不把 HTTP 插件侧车放进默认 compose
- `baize start` 仍进程内拉 mock-ticket；不调用 Docker
- commit 中文：`type(scope): 说明`
- Windows 上 `git commit` 不要用 bash HEREDOC，用 `git commit -m "..."` 
- Go：`C:\Users\Administrator\sdk\go\bin`；`GOPROXY=https://goproxy.cn,direct`

---

## 文件结构（将创建/修改）

| 路径 | 职责 |
|------|------|
| `examples/mock-ticket/listen.go` | `ListenAddr()`：默认 `:18080`，读 `BAIZE_MOCK_TICKET_LISTEN` |
| `examples/mock-ticket/listen_test.go` | 环境变量覆盖 |
| `examples/mock-ticket/cmd/mock-ticket/main.go` | 独立进程入口 |
| `scripts/start.sh` | POSIX 一键 `baize start` |
| `configs/docker.yaml` | Compose 试用配置 |
| `internal/config/docker_yaml_test.go` | 断言 docker.yaml 字段；回归 default.yaml |
| `Dockerfile` | 多阶段构建两二进制 |
| `docker-compose.yml` | baize + mock-ticket，无 depends_on |
| `.dockerignore` | 排除 data/.env/.git 等 |
| `README.md` / `README.zh-CN.md` | 三路径快速开始 + Docker 节 |

---

### 任务 1：mock-ticket 独立进程

**文件：**
- 创建：`examples/mock-ticket/listen.go`
- 创建：`examples/mock-ticket/listen_test.go`
- 创建：`examples/mock-ticket/cmd/mock-ticket/main.go`

- [ ] **步骤 1：编写失败的测试**

```go
package mockticket

import (
	"os"
	"testing"
)

func TestListenAddrDefault(t *testing.T) {
	t.Setenv("BAIZE_MOCK_TICKET_LISTEN", "")
	if got := ListenAddr(); got != ":18080" {
		t.Fatalf("ListenAddr()=%q want :18080", got)
	}
}

func TestListenAddrEnvOverride(t *testing.T) {
	t.Setenv("BAIZE_MOCK_TICKET_LISTEN", ":19091")
	if got := ListenAddr(); got != ":19091" {
		t.Fatalf("ListenAddr()=%q", got)
	}
}
```

空环境变量：实现里 `os.Getenv` 非空才覆盖，`t.Setenv(..., "")` 后应走默认。

- [ ] **步骤 2：** `$env:PATH = "C:\Users\Administrator\sdk\go\bin;" + $env:PATH; $env:GOPROXY = "https://goproxy.cn,direct"; go test ./examples/mock-ticket/ -count=1`  
  预期：FAIL（`ListenAddr` 未定义）

- [ ] **步骤 3：最少实现**

`listen.go`：

```go
package mockticket

import "os"

func ListenAddr() string {
	if v := os.Getenv("BAIZE_MOCK_TICKET_LISTEN"); v != "" {
		return v
	}
	return ":18080"
}
```

`cmd/mock-ticket/main.go`（对齐 `examples/http-plugin/cmd/http-plugin/main.go`）：

```go
package main

import (
	"log"
	"net/http"

	mockticket "github.com/rebornace/baize/examples/mock-ticket"
)

func main() {
	addr := mockticket.ListenAddr()
	log.Printf("mock-ticket listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mockticket.NewHandler()))
}
```

不要改 `internal/bootstrap` 的进程内 mock-ticket。

- [ ] **步骤 4：** `go test ./examples/mock-ticket/ -count=1` 通过；`go build -o NUL ./examples/mock-ticket/cmd/mock-ticket` 与 `go build -o NUL ./cmd/baize` 成功（Windows 用 `-o NUL`）

- [ ] **步骤 5：Commit** `feat(examples): mock-ticket 独立进程入口`

---

### 任务 2：POSIX start.sh

**文件：**
- 创建：`scripts/start.sh`

- [ ] **步骤 1：写脚本（无可单独 FAIL 的 Go 测试；用 `sh -n` 当红绿）**

完整内容：

```sh
#!/bin/sh
# Baize local one-shot launcher (POSIX).
# Usage (from repo root): ./scripts/start.sh
set -e

ROOT=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
cd "$ROOT"

if [ -z "${GOPROXY:-}" ]; then
	GOPROXY=https://goproxy.cn,direct
	export GOPROXY
fi
if [ -z "${GOSUMDB:-}" ]; then
	GOSUMDB=sum.golang.google.cn
	export GOSUMDB
fi

if ! command -v go >/dev/null 2>&1; then
	echo "go not found. Install Go 1.22+ or put it on PATH." >&2
	exit 1
fi

echo "baize start  (cwd=$ROOT  go=$(command -v go))"
exec go run ./cmd/baize start "$@"
```

不要写入 Windows `sdk\go` 路径。

- [ ] **步骤 2：语法检查**

优先：`D:\Git\bin\bash.exe -n scripts/start.sh` 或 `sh -n scripts/start.sh`。退出码 0。

- [ ] **步骤 3：可执行位**

```
git add --chmod=+x scripts/start.sh
```

若 `--chmod` 不可用：`git update-index --chmod=+x scripts/start.sh`。`git ls-files -s scripts/start.sh` 的 mode 应为 `100755`。

- [ ] **步骤 4：Commit** `feat(scripts): POSIX 一键启动 start.sh`

---

### 任务 3：docker.yaml + Dockerfile + compose

**文件：**
- 创建：`configs/docker.yaml`
- 创建：`internal/config/docker_yaml_test.go`
- 创建：`Dockerfile`
- 创建：`docker-compose.yml`
- 创建：`.dockerignore`

- [ ] **步骤 1：失败的测试**

```go
package config_test

func TestDockerYAMLSidecarCompose(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "docker.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MockTicket.Listen != "off" {
		t.Fatalf("mock_ticket.listen=%q want off", cfg.MockTicket.Listen)
	}
	if cfg.Connector.BaseURL != "http://mock-ticket:18080" {
		t.Fatalf("base_url=%q", cfg.Connector.BaseURL)
	}
	if cfg.Store.SQLitePath != "/app/data/baize.db" {
		t.Fatalf("sqlite=%q", cfg.Store.SQLitePath)
	}
	if cfg.Connector.Spec != "examples/mock-ticket/openapi.yaml" {
		t.Fatalf("spec=%q", cfg.Connector.Spec)
	}
	if cfg.LLM.Provider != "mock" || cfg.Agent.ID != "ticket-agent" {
		t.Fatalf("llm/agent %+v %+v", cfg.LLM, cfg.Agent)
	}
}

func TestDefaultYAMLUnchangedForLocalStart(t *testing.T) {
	cfg, err := config.Load(filepath.Join("..", "..", "configs", "default.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MockTicket.Listen != ":18080" {
		t.Fatalf("default mock listen=%q", cfg.MockTicket.Listen)
	}
	if cfg.Connector.BaseURL != "http://127.0.0.1:18080" {
		t.Fatalf("default base_url=%q", cfg.Connector.BaseURL)
	}
	if cfg.Connector.Type != "openapi" {
		t.Fatalf("type=%q", cfg.Connector.Type)
	}
}
```

补全 imports：`path/filepath`、`testing`、`github.com/rebornace/baize/internal/config`。

- [ ] **步骤 2：** `go test ./internal/config/ -count=1 -run DockerYAML` 预期 FAIL（文件不存在）

- [ ] **步骤 3：`configs/docker.yaml`**

从 `configs/default.yaml` 复制，只改这三处：

```yaml
store:
  driver: sqlite
  sqlite_path: /app/data/baize.db
connector:
  # 其余字段与 default.yaml 相同
  base_url: http://mock-ticket:18080
mock_ticket:
  listen: "off"
```

`listen: ":8080"`、`require_approval`、`auth`、agent、llm mock、`spec: examples/mock-ticket/openapi.yaml` 保持与 default 一致。不要写 API key。

- [ ] **步骤 4：Dockerfile**

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS build
WORKDIR /src
ARG GOPROXY=https://goproxy.cn,direct
ENV GOPROXY=${GOPROXY}
ENV CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /out/baize ./cmd/baize \
 && go build -o /out/mock-ticket ./examples/mock-ticket/cmd/mock-ticket

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=build /out/baize /app/baize
COPY --from=build /out/mock-ticket /app/mock-ticket
COPY configs/docker.yaml /app/configs/docker.yaml
COPY examples/mock-ticket/openapi.yaml /app/examples/mock-ticket/openapi.yaml
EXPOSE 8080
CMD ["/app/baize", "serve", "-config", "/app/configs/docker.yaml"]
```

- [ ] **步骤 5：docker-compose.yml**

```yaml
services:
  mock-ticket:
    build: .
    command: ["/app/mock-ticket"]
    ports:
      - "18080:18080"
  baize:
    build: .
    ports:
      - "8080:8080"
    volumes:
      - baize-data:/app/data
volumes:
  baize-data:
```

**不要**写 `depends_on`。baize 用镜像默认 CMD。

- [ ] **步骤 6：`.dockerignore`**

```
.git
.cursor
.superpowers
.worktrees
data
.env
.env.*
configs/default.local.yaml
web/chat/node_modules
*.exe
*.test
baize-stdout.log
baize-stderr.log
```

- [ ] **步骤 7：** `go test ./internal/config/ -count=1` 通过。若本机有 Docker：`docker compose config` 退出 0。没有 Docker 不视为失败。

- [ ] **步骤 8：Commit** `feat(deploy): Docker 镜像与 compose 试用栈`

---

### 任务 4：README 中英

**文件：**
- 修改：`README.md`（约 38–53 行「Quick start」、文末 Commands 前插入 Docker 节）
- 修改：`README.zh-CN.md` 对等位置

- [ ] **步骤 1：改 Quick start**

把现在的「**Windows (one-shot launcher):** / `.\start.cmd`」从加粗主路径拿掉。改为：

```markdown
## Quick start

**Requirements:** Go 1.22+ (no C compiler; SQLite is pure Go)

```bash
git clone https://github.com/rebornace/baize.git
cd baize
go run ./cmd/baize start
```

Same command on Linux, macOS, and Windows. Optional launchers (set `GOPROXY` when unset):

- POSIX: `./scripts/start.sh`
- Windows: `.\start.cmd`

### Native binary

```bash
# Linux server
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o baize ./cmd/baize
# macOS
CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -o baize ./cmd/baize
# Windows
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o baize.exe ./cmd/baize
```

Copy the binary to the host; no runtime besides the OS. See [Server (Docker)](#server-docker) for containers.
```

默认样板端口列表（8080 / 18080 / mock LLM / `/ui`）保留在这段之后。

中文对等：标题「快速开始」「本机二进制」，锚点「服务端（Docker）」。

- [ ] **步骤 2：新节 Server (Docker)**

放在 Chat UI 节之前（Commands 之前亦可，但须在 Quick start 能链到）。英文标题 `## Server (Docker)`，中文 `## 服务端（Docker）`。

```markdown
## Server (Docker)

Baize is a single static binary. On a Linux host, Docker is the supported server path.

**Try the sample stack** (Runtime + mock-ticket, mock LLM):

```bash
docker compose up --build
```

- Runtime / UI: http://127.0.0.1:8080  (`/ui`)
- Mock ticket API: http://127.0.0.1:18080

Start **both** services. `configs/docker.yaml` points `base_url` at hostname `mock-ticket`; running only `baize` with that file will fail connector registration.

**Production sidecar** (your APIs, no mock-ticket):

```bash
docker build -t baize:local .
docker run --rm -p 8080:8080 \
  -v /path/to/your.yaml:/app/configs/docker.yaml \
  -v baize-data:/app/data \
  -e BAIZE_API_KEY \
  baize:local
```

In `your.yaml` set `mock_ticket.listen: off` and `connector.base_url` to the real system. Do not use this repo's compose file as production orchestration.

Override module proxy at build time if needed: `docker build --build-arg GOPROXY=https://proxy.golang.org,direct .`
```

中文平行翻译。不要介绍 `export-public.ps1`。不要把 HTTP 插件放进 compose。

- [ ] **步骤 3：** 确认 `configs/default.yaml` 未改。`go test ./... -count=1`

- [ ] **步骤 4：Commit** `docs(deploy): 三系统启动与 Docker 服务端说明`

---

## 自检（对照规格）

| 规格要点 | 任务 |
|----------|------|
| README 三系统同一套 go run/build；Windows 非主入口 | 4 |
| start.sh GOPROXY / 缺 go 退出 | 2 |
| GOOS 交叉编译示例、无 CGO | 4 |
| compose 8080+18080 | 3 |
| docker run 生产、不强制 mock-ticket | 3、4 |
| default.yaml 不变 | 3 回归测试、4 确认 |
| mock-ticket cmd | 1 |
| docker.yaml / Dockerfile / compose / dockerignore | 3 |
| 不做 CI/systemd/goreleaser | 全计划未包含 |

无 TODO 占位。镜像内路径：`/app/baize`、`/app/mock-ticket`、`/app/configs/docker.yaml`。环境变量：`BAIZE_MOCK_TICKET_LISTEN`。

---

## 执行交接

计划已保存到 `docs/superpowers/plans/2026-08-14-cross-platform-deploy.md`。

**两种执行方式：**

1. **子代理驱动（推荐）** — 每任务新子代理 + 任务间审查
2. **内联执行** — 本会话用 executing-plans 按任务推进并设检查点

选哪种方式？
