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
  replicas: {
    id: string
    age_sec: number
    goroutines: number
  }[]
  replica_count: number
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

export interface BudgetAlertEntry {
  at: string
  target?: string
  target_id?: string
  scope: string
  period: string
  level: string
  total_usd: number
  budget_usd: number
  ratio: number
}

export interface AuditEvent {
  id: string
  operator_id: string
  role: string
  method: string
  action: string
  target: string
  status: number
  remote_addr?: string
  created_at: string
}

export interface AuditFilters {
  q?: string
  operator_id?: string
  action?: string
  from?: string
  to?: string
}

function auditQuery(filters: AuditFilters): string {
  const qs = new URLSearchParams()
  for (const [key, value] of Object.entries(filters)) {
    if (value) qs.set(key, value)
  }
  return qs.toString()
}

export async function getAuditEvents(
  limit = 50,
  filters: AuditFilters = {},
): Promise<{ items: AuditEvent[]; limit: number }> {
  const qs = auditQuery(filters)
  const suffix = qs ? `&${qs}` : ''
  const res = await fetch(`/v0/audit/events?limit=${limit}${suffix}`, { headers: authInit() })
  return parseJSON(res)
}

export interface AuditRetention {
  rows: number
  min: number
  max: number
}

export async function getAuditRetention(): Promise<AuditRetention> {
  const res = await fetch('/v0/settings/audit-retention', { headers: authInit() })
  return parseJSON<AuditRetention>(res)
}

export async function putAuditRetention(rows: number): Promise<{ rows: number }> {
  const res = await fetch('/v0/settings/audit-retention', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ rows }),
  })
  return parseJSON(res)
}

/** Download the filtered audit trail as CSV using the authenticated API. */
export async function exportAuditCSV(filters: AuditFilters = {}): Promise<void> {
  const qs = auditQuery(filters)
  const res = await fetch(`/v0/audit/export${qs ? `?${qs}` : ''}`, { headers: authInit() })
  if (!res.ok) {
    let message = res.statusText
    try {
      const body = (await res.json()) as { error?: { message?: string } }
      if (body.error?.message) message = body.error.message
    } catch {
      /* ignore non-JSON */
    }
    throw new ApiError(res.status, 'audit_export_failed', `HTTP ${res.status}: ${message}`)
  }
  const blob = await res.blob()
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = `baize-audit-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, '')}.csv`
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

export async function getBudgetAlertHistory(
  limit = 50,
): Promise<{ items: BudgetAlertEntry[]; limit: number }> {
  const res = await fetch(`/v0/settings/budget-alerts?limit=${limit}`, { headers: authInit() })
  return parseJSON(res)
}

export interface BudgetAlertSilence {
  until: string | null
  reason: string
  active: boolean
}

export async function getBudgetAlertSilence(): Promise<BudgetAlertSilence> {
  const res = await fetch('/v0/settings/budget-alert-silence', { headers: authInit() })
  return parseJSON(res)
}

export async function setBudgetAlertSilence(
  until: string,
  reason = '',
): Promise<BudgetAlertSilence> {
  const res = await fetch('/v0/settings/budget-alert-silence', {
    method: 'PUT',
    headers: authInit({ 'Content-Type': 'application/json' }),
    body: JSON.stringify({ until, reason }),
  })
  return parseJSON(res)
}

export interface CostAnomalyEntry {
  at: string
  scope: string
  conversation_id?: string
  today_usd: number
  baseline_usd: number
  ratio: number
}

export async function getCostAnomalies(
  limit = 50,
): Promise<{ items: CostAnomalyEntry[]; limit: number }> {
  const res = await fetch(`/v0/settings/cost-anomalies?limit=${limit}`, { headers: authInit() })
  return parseJSON(res)
}

/** Fetch recent process log lines (Admin). */
export async function getRecentLogs(limit = 200): Promise<RecentLogsResponse> {
  const res = await fetch(`/v0/settings/logs?limit=${limit}`, { headers: authInit() })
  return parseJSON(res)
}
