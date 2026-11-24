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

/** Fetch recent process log lines (Admin). */
export async function getRecentLogs(limit = 200): Promise<RecentLogsResponse> {
  const res = await fetch(`/v0/settings/logs?limit=${limit}`, { headers: authInit() })
  return parseJSON(res)
}
