// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { buildMetricsOverview, formatDurationSeconds, parsePromSamples } from './metricsParse'

const SAMPLE = `
# HELP baize_runs_total Terminal run count by status.
# TYPE baize_runs_total counter
baize_runs_total{status="succeeded"} 10
baize_runs_total{status="failed"} 2
# TYPE baize_run_duration_seconds histogram
baize_run_duration_seconds_sum 25
baize_run_duration_seconds_count 12
baize_run_duration_seconds_bucket{le="0.005"} 0
baize_tool_invocations_total{connector_id="shell",result="ok"} 7
baize_tool_invocations_total{connector_id="shell",result="error"} 1
baize_mcp_export_invocations_total{result="ok"} 3
`

describe('metricsParse', () => {
  it('parses labeled samples and skips help/type/buckets', () => {
    const samples = parsePromSamples(SAMPLE)
    expect(samples.some((s) => s.name.endsWith('_bucket'))).toBe(false)
    expect(samples.find((s) => s.name === 'baize_runs_total' && s.labels.status === 'succeeded')?.value).toBe(
      10,
    )
  })

  it('builds overview totals', () => {
    const o = buildMetricsOverview(SAMPLE)
    expect(o.runsTotal).toBe(12)
    expect(o.runsByStatus).toEqual([
      { status: 'succeeded', count: 10 },
      { status: 'failed', count: 2 },
    ])
    expect(o.avgRunSeconds).toBeCloseTo(25 / 12)
    expect(o.toolOk).toBe(7)
    expect(o.toolError).toBe(1)
    expect(o.mcpOk).toBe(3)
    expect(o.mcpError).toBe(0)
  })

  it('formats short durations', () => {
    expect(formatDurationSeconds(0.12)).toBe('120 ms')
    expect(formatDurationSeconds(3.2)).toBe('3.2 s')
  })
})
