[中文](./deployment.md) | **English**

# Deployment: Runtime and channel adapters

Baize is a single static Go binary. **Message channels** are extensible and connect through sidecar adapter processes; the adapter this repository ships today is the personal WeChat (iLink) channel **`weixin-adapter`** (`cmd/weixin-adapter`). Baize and the adapter exchange HMAC-signed JSON over HTTP:

| Direction | Path |
|------|------|
| Adapter → baize | `POST /v0/channels/{name}/inbound` |
| baize → adapter | `/outbound` (send messages), `/admin/*` (login / status / start / stop / shutdown), `GET /healthz` |

Pick one mode per host: **Autostart** or **Standalone**.

## Autostart (desktop / single host, including Windows)

baize launches and supervises the adapter subprocess: exponential backoff restart on crash; graceful shutdown of the child on exit. Leave `secret` empty to auto-generate and persist a shared HMAC. `creds.json` defaults under `adapter_creds_dir` (for example `./data/channels/weixin`).

Build the adapter first (baize looks on `PATH` or under `./bin/`):

```bash
go build -o bin/weixin-adapter ./cmd/weixin-adapter   # Windows: bin/weixin-adapter.exe
# then start baize (Windows: .\serve.cmd; macOS / Linux: ./scripts/serve.sh)
```

Sample channel block (aligned with `configs/config.yaml`: **dynamic port by default**; do not put a fixed `-addr` in `adapter_args`; `outbound_url` / `admin_url` are placeholders overwritten after start via `credsDir/listen.port`):

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

For a fixed port, put a non-zero `-addr` in `adapter_args` (for example `-addr=127.0.0.1:8090`). In this mode **do not** also enable `weixin-adapter.service`—the child belongs to baize.

## Standalone (production Linux / containers)

The adapter runs as its own service; baize does **not** spawn a child. Use systemd `Restart=always` or compose `restart: unless-stopped` as the watchdog. The shared secret must be configured explicitly on both sides.

### baize channel config

`adapter_autostart: "false"`; do not set `adapter_command` / `adapter_args`:

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
      outbound_secret: "<shared-secret>"   # omit to fall back to secret
```

### Adapter flags

| Flag | Meaning | Default |
|------|------|------|
| `-baize` | baize inbound URL (required) | — |
| `-secret` | shared HMAC (required) | — |
| `-addr` | listen (admin / outbound / healthz) | `127.0.0.1:8090` |
| `-creds` | directory for `creds.json` | `./data/channels/weixin` |
| `-ilink-base` | override iLink API (testing) | — |
| `-port-file` | write dynamic port to a file (autostart discovery) | — |

### Docker Compose

Release images include `/app/baize` and `/app/weixin-adapter`. [`docker-compose.weixin.yml`](../../docker-compose.weixin.yml) starts both services and mounts [`configs/docker-weixin-standalone.yaml`](../../configs/docker-weixin-standalone.yaml):

```bash
export BAIZE_API_KEY=sk-...
export WEIXIN_ADAPTER_SECRET='<shared-secret>'   # production: strong random; must match channel secret
docker compose -f docker-compose.weixin.yml up --build
```

Conventions:

- Volume `baize-data` → `/app/data`; `weixin-data` → `/data/channels/weixin`
- baize → `http://weixin-adapter:8090`; adapter inbound → `http://baize:8080/v0/channels/weixin/inbound`
- Sample default shared secrets are for trial only; production must change both the env and YAML (or mount your own config)

### systemd

Units live under [`deploy/systemd/`](../../deploy/systemd/). You can also download a prebuilt `linux` archive from [GitHub Releases](https://github.com/rebornace/baize/releases), unpack under `/opt/baize`, and skip the `go build` lines below:

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /opt/baize/baize ./cmd/baize
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /opt/baize/weixin-adapter ./cmd/weixin-adapter
cp deploy/systemd/baize.service deploy/systemd/weixin-adapter.service /etc/systemd/system/
# Edit weixin-adapter.service: REPLACE_WITH_SHARED_SECRET must match the channel secret
systemctl daemon-reload
systemctl enable --now baize.service
systemctl enable --now weixin-adapter.service   # Standalone only
```

- `baize.service`: `baize serve -config /opt/baize/config.yaml`, `Restart=always`
- Autostart mode: enable only `baize.service`

## Common notes

- **Shared secret**: Autostart can auto-generate; Standalone requires both sides to match (channel `secret` / `outbound_secret` and adapter `-secret`).
- **Credentials directory**: Persist `-creds`; QR login once per deployment is enough. One account maps to one adapter instance—do not share the same credentials directory across processes.
- **Graceful shutdown**: baize handles SIGINT/SIGTERM; under Autostart it stops the child via `/admin/shutdown` → SIGTERM → force kill. Under Standalone the adapter is managed by its own orchestrator.
