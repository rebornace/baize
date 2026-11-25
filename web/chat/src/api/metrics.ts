import { ApiError, authInit, parseJSON } from './http'

/** Fetch Prometheus text from GET /metrics (Admin Bearer when gate is on). */
export async function getMetricsText(): Promise<string> {
  const res = await fetch('/metrics', { headers: authInit() })
  if (!res.ok) {
    let code = 'unknown'
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: { code?: string; message?: string } }
      if (body.error?.code) code = body.error.code
      if (body.error?.message) message = body.error.message
    } catch {
      /* ignore non-JSON */
    }
    throw new ApiError(res.status, code, `HTTP ${res.status}: ${message}`)
  }
  return res.text()
}

export interface RecentLogsResponse {
  lines: string[]
  limit: number
}

export interface RunOverviewError {
  message: string
  count: number
}

export interface RunOverview {
  days: number
  total: number
  by_status: Record<string, number>
  terminal: number
  success_rate: number
  active: number
  waiting_human: number
  range_cost_usd: number
  today_cost_usd: number
  top_errors: RunOverviewError[]
  truncated: boolean
}

export async function getRunOverview(days = 7): Promise<RunOverview> {
  const res = await fetch(`/v0/settings/run-overview?days=${days}`, { headers: authInit() })
  return parseJSON<RunOverview>(res)
}

export interface OpsStatus {
  runs: {
    queued: number
    running: number
    waiting_human: number
    active: number
    succeeded_24h: number
    failed_24h: number
  }
  process: {
    goroutines: number
  }
  db: {
    max_open: number
    open: number
    in_use: number
    idle: number
    wait_count: number
  }
  streams: {
    live: number
    dropped: number
    slow: number
  }
  webhook_configured: boolean
  budget: {
    day_alert: string
    month_alert: string
    scoped_alerts: number
  }
  eval_gate: {
    last_pass: boolean | null
    last_ran_at: string
    total: number
    failed: number
  }
  retention: {
    attachment_days: number
    run_days: number
    message_days: number
  }
}

export async function getOpsStatus(): Promise<OpsStatus> {
  const res = await fetch('/v0/settings/ops-status', { headers: authInit() })
  return parseJSON<OpsStatus>(res)
}

/** Fetch recent process log lines (Admin). */
export async function getRecentLogs(limit = 200): Promise<RecentLogsResponse> {
  const res = await fetch(`/v0/settings/logs?limit=${limit}`, { headers: authInit() })
  return parseJSON(res)
}
