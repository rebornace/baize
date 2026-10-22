/** Minimal Prometheus text exposition sample parser (baize metrics only). */

export type PromSample = {
  name: string
  labels: Record<string, string>
  value: number
}

export type MetricsOverview = {
  runsByStatus: { status: string; count: number }[]
  runsTotal: number
  avgRunSeconds: number | null
  toolOk: number
  toolError: number
  mcpOk: number
  mcpError: number
}

const SAMPLE_RE =
  /^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{(.*)\})?\s+([+-]?(?:\d+\.?\d*|\.\d+)(?:[eE][+-]?\d+)?|NaN|Inf|\+Inf|-Inf)\s*$/

function parseLabels(raw: string | undefined): Record<string, string> {
  if (!raw?.trim()) return {}
  const out: Record<string, string> = {}
  const re = /([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"\\])*)"/g
  let m: RegExpExecArray | null
  while ((m = re.exec(raw)) != null) {
    out[m[1]] = m[2].replace(/\\"/g, '"').replace(/\\\\/g, '\\')
  }
  return out
}

function parseNumber(raw: string): number {
  if (raw === 'NaN') return Number.NaN
  if (raw === '+Inf' || raw === 'Inf') return Number.POSITIVE_INFINITY
  if (raw === '-Inf') return Number.NEGATIVE_INFINITY
  return Number(raw)
}

export function parsePromSamples(text: string): PromSample[] {
  const samples: PromSample[] = []
  for (const line of text.split(/\r?\n/)) {
    const trimmed = line.trim()
    if (!trimmed || trimmed.startsWith('#')) continue
    const m = SAMPLE_RE.exec(trimmed)
    if (!m) continue
    const name = m[1]
    if (name.endsWith('_bucket') || name.endsWith('_created')) continue
    const value = parseNumber(m[4])
    if (!Number.isFinite(value)) continue
    samples.push({ name, labels: parseLabels(m[3]), value })
  }
  return samples
}

export function buildMetricsOverview(text: string): MetricsOverview {
  const samples = parsePromSamples(text)
  const runsMap = new Map<string, number>()
  let runSum = 0
  let runCount = 0
  let toolOk = 0
  let toolError = 0
  let mcpOk = 0
  let mcpError = 0

  for (const s of samples) {
    if (s.name === 'baize_runs_total') {
      const status = s.labels.status || 'unknown'
      runsMap.set(status, (runsMap.get(status) ?? 0) + s.value)
    } else if (s.name === 'baize_run_duration_seconds_sum') {
      runSum += s.value
    } else if (s.name === 'baize_run_duration_seconds_count') {
      runCount += s.value
    } else if (s.name === 'baize_tool_invocations_total') {
      if (s.labels.result === 'error') toolError += s.value
      else toolOk += s.value
    } else if (s.name === 'baize_mcp_export_invocations_total') {
      if (s.labels.result === 'error') mcpError += s.value
      else mcpOk += s.value
    }
  }

  const runsByStatus = [...runsMap.entries()]
    .map(([status, count]) => ({ status, count }))
    .sort((a, b) => b.count - a.count || a.status.localeCompare(b.status))

  const runsTotal = runsByStatus.reduce((n, r) => n + r.count, 0)

  return {
    runsByStatus,
    runsTotal,
    avgRunSeconds: runCount > 0 ? runSum / runCount : null,
    toolOk,
    toolError,
    mcpOk,
    mcpError,
  }
}

export function formatDurationSeconds(seconds: number): string {
  if (seconds < 1) return `${Math.round(seconds * 1000)} ms`
  if (seconds < 60) return `${seconds.toFixed(seconds < 10 ? 1 : 0)} s`
  const m = Math.floor(seconds / 60)
  const s = Math.round(seconds % 60)
  return `${m} m ${s} s`
}
