# Inbox alert example

Minimal end-to-end story: an external monitor (or this script) POSTs a signed alert to Baize **Webhook Inbox v1**, which starts an Agent Run. If you configure an outbound run-event Webhook (global or per-channel), Baize POSTs run events back to your platform.

## Prerequisites

1. Baize Runtime running (e.g. `.\demo.cmd` or `go run ./cmd/baize demo`).
2. An Inbox channel (default id `alerts`) bound to an Agent — e.g. **Settings → Inbox** in `/ui`, or seed via YAML:

   ```yaml
   inbox:
     channels:
       - id: alerts
         agent_id: ticket-agent
         enabled: true
         skills: [ticket-triage]
   ```

3. Channel **secret** — shown once when you create the channel or rotate the secret (**Settings → Inbox → Rotate secret**).

## Send a signed alert (PowerShell)

```powershell
$env:RUNTIME_URL = "http://127.0.0.1:8080"
$env:INBOX_SECRET = "<paste-channel-secret>"
.\examples\inbox-alert\post.ps1
```

Optional channel id (default `alerts`):

```powershell
.\examples\inbox-alert\post.ps1 -ChannelId alerts
```

On success you should see **202 Accepted** (or **200 OK** on idempotent replay) with JSON like:

```json
{
  "delivery_id": "dlv_...",
  "run_id": "run_...",
  "conversation_id": "...",
  "status": "accepted"
}
```

Poll the run:

```bash
curl -s http://127.0.0.1:8080/v0/runs/<run_id>
curl -s http://127.0.0.1:8080/v0/runs/<run_id>/events
```

The first event should include `inbox.received` with `channel_id`, `delivery_id`, and any `external_id` / `metadata` you sent.

When the run reaches `waiting_human`, you can approve or reject via the **same signed POST** with `action: "resume"`, `run_id`, and `decision` (`approve` | `reject`) — same HMAC headers as create. See [README.zh-CN — 机器审批（HITL resume）](../../README.zh-CN.md#机器审批hitl-resume).

## Signing (v1)

| Item | Value |
|------|-------|
| Endpoint | `POST /v0/inbox/{channel_id}` |
| Timestamp header | `X-Baize-Inbox-Timestamp` — Unix seconds; skew ≤ 300s |
| Signature header | `X-Baize-Inbox-Signature: v1=<hex>` |
| Algorithm | `HMAC-SHA256(secret, "<timestamp>.<raw_body_bytes>")` |

Inbox uses **channel secret + HMAC only** — no Operator Token on this path (same as plugin callbacks).

## See also

- [README — Production integration: Webhook Inbox](../../README.md#production-integration-webhook-inbox)
- [README.zh-CN — 生产集成：Webhook Inbox](../../README.zh-CN.md#生产集成webhook-inbox)
