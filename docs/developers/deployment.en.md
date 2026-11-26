[中文](./deployment.md) | **English**

# Deployment: Runtime and channel adapters

Baize is a single static Go binary. **Message channels** are extensible and connect through sidecar adapter processes; the **production adapter** in this repo is the personal WeChat (iLink) channel **`weixin-adapter`** (`cmd/weixin-adapter`). Feishu / DingTalk **reference adapters** live under `examples/im-adapter-feishu` and `examples/im-adapter-dingtalk` (same webhook inbound/outbound contract; not bundled production processes). Baize and the adapter exchange HMAC-signed JSON over HTTP:

| Direction | Path |
|------|------|
| Adapter → baize | `POST /v0/channels/{name}/inbound` |
| baize → adapter | `/outbound` (send messages), `/admin/*` (login / status / start / stop / shutdown), `GET /healthz` |

After signature verification the inbound endpoint returns **202 Accepted** immediately: the raw message is persisted first, then a background worker resolves attachments / creates the Run with retries. Adapters no longer wait for attachment downloads. A repeated `idempotency_key` returns **200 `duplicate`**; invalid signatures still return 401.

Messages that exhaust their retries appear under **Settings → Inbound → Failed inbound messages**, where an admin can inspect the error and retry. The list never exposes the raw message body.

Pick one mode per host: **Autostart** or **Standalone**.

## Docker image

The repository `Dockerfile` is a multi-stage build: Alpine runtime, non-root user `baize`, `/app/data` volume, and a `HEALTHCHECK` against `/healthz`. The image ships both `baize` and `weixin-adapter`.

```bash
docker build -t baize:0.34.0 .
docker run -d --name baize -p 8080:8080 \
  -e BAIZE_API_KEY=sk-... \
  -e BAIZE_ADMIN_TOKEN='<admin-token>' \
  -v baize-data:/app/data \
  baize:0.34.0
```

Or use compose (`BAIZE_API_KEY` is required):

```bash
export BAIZE_API_KEY=sk-...
export BAIZE_ADMIN_TOKEN='<admin-token>'       # optional
export BAIZE_SETTINGS_KEY='<32-byte-secret>'   # required to persist encrypted settings
docker compose up -d --build
docker compose ps                              # STATUS should be healthy
```

Notes:

- The container runs as `baize` (non-root); `/app/data` is the only directory that needs persistence.
- The health check calls `http://127.0.0.1:8080/healthz` every 30s with a 15s startup grace period.
- The image health check keeps liveness semantics; behind a load balancer with multiple replicas, use `/readyz` as the readiness probe so store failures return 503.
- Without `BAIZE_SETTINGS_KEY`, encrypted settings such as API keys and channel secrets cannot be stored.
- The bundled `configs/docker-minimal.yaml` uses SQLite at `/app/data/baize.db`.

Multi-arch and supply chain:

```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t baize:0.34.0-multi \
  --output type=oci,dest=baize-oci.tar \
  --sbom=true --provenance=mode=min .
```

- The OCI archive contains `linux/amd64` and `linux/arm64` manifests plus SPDX SBOM and SLSA provenance.
- Scan with Trivy or Docker Scout before shipping; do not publish with CRITICAL findings.
- `apk upgrade` is part of the image build; base image and Go toolchain still need security updates.

### Signing release artifacts (optional)

Publishers can sign `SHA256SUMS.txt` and the OCI archive with a local cosign key. The private key stays offline; publish only `cosign.pub` and the `*.sigstore.json` bundles:

```bash
cosign verify-blob --key cosign.pub \
  --bundle SHA256SUMS.txt.sigstore.json \
  --offline SHA256SUMS.txt
```

Script: `scripts/sign-release.ps1 -Key <cosign.key> -ArtifactsDir <release-dir> -OciArchive <oci.tar>`. Signatures are uploaded to the Rekor transparency log by default; verification uses the matching bundle.

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
