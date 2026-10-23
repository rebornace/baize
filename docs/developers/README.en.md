[中文](./README.md) | **English**

# Developer documentation

For contributors and integrators. Product overview and trial: [README (English)](../../README.md) · [中文](../../README.zh-CN.md). Marketing capability story: [rebornace.github.io/baize](https://rebornace.github.io/baize/en/).

Version detail lives in [CHANGELOG](../../CHANGELOG.md). These capabilities are on the product surface (not Changelog-only); technical boundaries are in [Architecture](./architecture.en.md):

| Theme | Where users see it | Technical hook |
|-------|--------------------|----------------|
| Decision layer / tool narrowing | Settings → Matching & decisions; Runtime → Smart speed-up | Decision model, `toolindex`, decide knobs |
| Connector groups / group sub-tasks | Matching & decisions → Connector groups; Chat group sub-tasks | `connector_groups`, `spawn_group_run`, `waiting_child` |
| Hosted login & MCP export | External login; Expose capabilities | `/v0/auth/hosted/...`, MCP export, `identity_slots` |
| Catalog change diff & outbound alert | Connector save toast; `events.webhook` | `catalog_diff`, `type=catalog.diff` |
| Reconstructable runs | Chat “View run binding” | `run.binding`, `contract_hash` / `catalog_revision` |
| Schedules | Settings → Schedules | schedule store + timed new Runs |
| Context offload | Oversized tool results / pastes → workspace paths | conversation `offload/`, `read_file` |
| Eval corpus | Settings → Eval corpus (Admin) | EvalCase, evidence re-score |
| Ops observability | Settings → Runtime metrics; Runtime → Observability | `GET /metrics`, OTLP, slog hot-reload |
| Outbound mTLS | Connector editor → Outbound TLS / mTLS | `tls.cert_file` / `key_file` / `ca_file` |

| Doc | Contents |
|-----|----------|
| [Getting started](./getting-started.en.md) · [中文](./getting-started.md) | Build, test, CI, contributing |
| [Configuration](./configuration.en.md) · [中文](./configuration.md) | YAML / env / CLI |
| [HTTP API](./http-api.en.md) · [中文](./http-api.md) | Control-plane HTTP overview |
| [Architecture](./architecture.en.md) · [中文](./architecture.md) | Runtime boundaries, plugins, core abstractions |
| [Deployment](./deployment.en.md) · [中文](./deployment.md) | Single-host and channel adapter deploy |
| [Mobile](./mobile.en.md) · [中文](./mobile.md) | Remote console on phone, or custom UI via the `mobileagent` SDK |
| [Website (GitHub Pages)](../../site/README.md) | Static site source and enable notes |
| [Performance probes](./performance.en.md) · [中文](./performance.md) | Local hot-path reproduction |
