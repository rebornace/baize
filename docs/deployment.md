# Deployment guide

Baize is a single static Go binary. The WeChat (iLink) channel runs as a
separate process, `weixin-adapter` (`cmd/weixin-adapter`), which exchanges
HMAC-signed JSON with baize over HTTP:

- Adapter → baize: inbound messages POST to `/v0/channels/<name>/inbound`
- baize → adapter: `/outbound` (send messages), `/admin/*` (login / status /
  start / stop / shutdown), `/healthz`

There are two supported deployment modes. Pick one per host.

## Autostart mode (desktop / demo / single host, including Windows)

Baize launches and supervises the adapter child process itself.

- **Zero extra processes to manage** — one `baize` process; the adapter is its
  child.
- **Cross-platform watchdog** — if the adapter crashes, baize restarts it with
  exponential backoff; on shutdown baize terminates the child gracefully
  (SIGTERM, then kill after a grace period). Works on Windows, macOS, Linux.
- **Secret is automatic** — leave `secret` empty and baize generates a shared
  HMAC key, persisting it so an orphaned adapter from a previous run still
  verifies requests.
- Login credentials (`creds.json`) live under `adapter_creds_dir`
  (default `./data/channels/weixin`), so restarts need no re-scan.

This is what the sample configs use. Build the adapter binary first (baize
finds it via `PATH` or `./bin/`):

```bash
go build -o bin/weixin-adapter ./cmd/weixin-adapter      # Windows: bin/weixin-adapter.exe
go run ./cmd/baize demo                                   # or: .\demo.cmd on Windows
```

Channel config (`configs/demo.yaml` already ships this):

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
      adapter_args: "-addr=127.0.0.1:8090,-creds=./data/channels/weixin"
      adapter_creds_dir: ./data/channels/weixin
```

Best for: local demos, a developer laptop, a single Windows/Linux box. Do **not**
enable `weixin-adapter.service` (systemd) in this mode — baize owns the child.

## Standalone mode (production Linux / containers)

The adapter runs as its own service. Baize connects to it over HTTP and does
**not** spawn anything.

- **Process watchdog is external** — systemd `Restart=always` or a container
  restart policy (`restart: unless-stopped`) restarts a crashed adapter.
- **Logs** go to journald or the container's log driver; **resource limits**
  are set via systemd / cgroup. Baize intentionally does not implement log
  rotation or cgroup limits — that is the orchestrator's job (YAGNI).
- The shared secret must be configured explicitly on both sides.

### baize-side channel config

Set `adapter_autostart: "false"` and point at the adapter over the network.
Do **not** set `adapter_command` / `adapter_args` (those are autostart-only):

```yaml
channels:
  - name: weixin
    type: webhook
    enabled: true
    config:
      source: weixin
      adapter_autostart: "false"
      admin_url: http://<adapter-host>:8090          # compose: http://weixin-adapter:8090
      outbound_url: http://<adapter-host>:8090/outbound
      assignee: channel:weixin
      supports_vision: "true"
      secret: "<shared-secret>"                       # same value as the adapter's -secret
      outbound_secret: "<shared-secret>"              # defaults to secret if omitted
```

### Adapter flags

| Flag | Meaning | Default |
|------|---------|---------|
| `-baize` | baize inbound URL (required) | — |
| `-secret` | shared HMAC secret (required) | — |
| `-addr` | listen address (admin/outbound/healthz) | `127.0.0.1:8090` |
| `-creds` | directory for `creds.json` | `./data/channels/weixin` |
| `-ilink-base` | override iLink API base (testing) | — |

### Quickstart: Docker Compose

The released image contains both `/app/baize` and `/app/weixin-adapter`.
`docker-compose.weixin.yml` runs them as two services; compose restarts both
services (`restart: unless-stopped`), and the adapter's credentials persist in
a named volume. The compose file ships a ready-to-use baize config
(`configs/docker-weixin-standalone.yaml`, mounted into the container) that
already declares the standalone `weixin` webhook channel — so a single command
brings the whole stack up end-to-end with no extra config to prepare:

```bash
export BAIZE_API_KEY=sk-...
# Optional for trying it out locally; REQUIRED (strong random value) for
# production — it must match the channel config secret.
export WEIXIN_ADAPTER_SECRET='<shared-secret>'
docker compose -f docker-compose.weixin.yml up --build
```

Conventions used by the compose file:

- baize data (SQLite DB, artifacts): volume `baize-data` → `/app/data`
- adapter credentials: volume `weixin-data` → `/data/channels/weixin`
  (`-creds=/data/channels/weixin`), so a QR login survives container restarts
- baize reaches the adapter at `http://weixin-adapter:8090`; the adapter
  posts inbound to `http://baize:8080/v0/channels/weixin/inbound`
- baize starts with the bundled standalone config
  (`configs/docker-weixin-standalone.yaml`), bind-mounted read-only to
  `/app/configs/docker-weixin-standalone.yaml`; it sets
  `adapter_autostart: "false"`, `admin_url`/`outbound_url` pointing at the
  `weixin-adapter` service, and the shared `secret`/`outbound_secret`

**Shared secret (production must-do):** the bundled config and the compose
file share a development default `dev-standalone-weixin-shared-secret` so the
stack runs with zero setup for a trial. For production set BOTH sides to the
same strong random value: (1) `export WEIXIN_ADAPTER_SECRET=<strong-random>`
(the adapter's `-secret`; compose interpolates it), and (2) put the same value
in the baize channel config's `secret` and `outbound_secret` — edit
`configs/docker-weixin-standalone.yaml` in place (it is bind-mounted) or mount
your own config file over `/app/configs/docker-weixin-standalone.yaml`. Both
services also carry `restart: unless-stopped`.

### Quickstart: systemd (bare metal / VM)

Build the binaries and place them under `/opt/baize` with your `config.yaml`
(containing the standalone channel block):

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /opt/baize/baize ./cmd/baize
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /opt/baize/weixin-adapter ./cmd/weixin-adapter
```

Install the units from `deploy/systemd/`:

```bash
cp deploy/systemd/baize.service deploy/systemd/weixin-adapter.service /etc/systemd/system/
# Edit weixin-adapter.service: replace REPLACE_WITH_SHARED_SECRET with the
# same secret as the baize channel config.
systemctl daemon-reload
systemctl enable --now baize.service
systemctl enable --now weixin-adapter.service   # standalone mode only
journalctl -u weixin-adapter -f                 # logs
```

- `baize.service` runs `baize serve -config /opt/baize/config.yaml` with
  `Restart=always`.
- `weixin-adapter.service` has `Restart=always` and orders after
  `baize.service`; its credentials persist under
  `/opt/baize/data/channels/weixin`.
- In autostart mode, enable only `baize.service` — baize supervises the
  adapter itself.

## Shared conventions

- **Shared secret** — HMAC-SHA256 key for baize↔adapter traffic. Autostart:
  generated automatically when `secret` is empty. Standalone: you must set the
  same value in the channel config (`secret`, and `outbound_secret` if you
  want a separate outbound key) and in the adapter's `-secret` flag
  (`WEIXIN_ADAPTER_SECRET` in compose).
- **Credentials directory** — the adapter stores the iLink `creds.json` under
  `-creds` and reloads it at startup, so QR login is one-time per deployment.
  Always back this directory with a persistent volume / disk path.
- **Single bot account** — run one adapter instance per logged-in WeChat
  account; do not point two adapters at the same credentials directory.
- **Graceful shutdown** — baize traps SIGINT/SIGTERM (Ctrl-C, `docker stop`,
  `systemctl stop`): it drains the HTTP server and then runs the closer, which
  in autostart mode terminates the supervised adapter child gracefully
  (HMAC `/admin/shutdown` → SIGTERM → force kill) rather than orphaning it.
  In standalone mode the adapter is stopped/restarted independently by its own
  service manager.
