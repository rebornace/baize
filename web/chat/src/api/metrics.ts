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

/** Fetch recent process log lines (Admin). */
export async function getRecentLogs(limit = 200): Promise<RecentLogsResponse> {
  const res = await fetch(`/v0/settings/logs?limit=${limit}`, { headers: authInit() })
  return parseJSON(res)
}
