import { useEffect, useState } from 'react'
import { getRunOverview, type RunOverview } from '../api/metrics'
import { METRICS } from '../strings'

const DAYS = 7

export function RunOverviewCard() {
  const [data, setData] = useState<RunOverview | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const next = await getRunOverview(DAYS)
        if (!cancelled) {
          setData(next)
          setFailed(false)
        }
      } catch {
        if (!cancelled) setFailed(true)
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [])

  if (failed) {
    return <p className="settings-muted">{METRICS.overviewLoadFailed}</p>
  }
  if (!data) {
    return <p className="settings-muted">{METRICS.refreshing}</p>
  }
  if (data.total === 0) {
    return <p className="settings-muted">{METRICS.overviewEmpty}</p>
  }

  const pct = Math.round(data.success_rate * 100)
  return (
    <section className="run-overview" data-testid="run-overview">
      <div className="run-overview-head">
        <h2 className="settings-subheading">{METRICS.overviewTitle}</h2>
        <span className="settings-muted">{METRICS.overviewRange(data.days)}</span>
      </div>
      <div className="run-overview-grid">
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.overviewSuccess}</span>
          <strong>{pct}%</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.overviewTotal}</span>
          <strong>{data.total}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.overviewFailed}</span>
          <strong>{data.by_status.failed ?? 0}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.overviewWaiting}</span>
          <strong>{data.waiting_human}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.overviewCost}</span>
          <strong>{data.range_cost_usd.toFixed(4)}</strong>
        </div>
      </div>
      {data.truncated ? (
        <p className="settings-muted">{METRICS.overviewTruncated(500)}</p>
      ) : null}
      {data.top_errors.length > 0 ? (
        <div className="run-overview-errors">
          <span className="settings-muted">{METRICS.overviewTopErrors}</span>
          <ul>
            {data.top_errors.map((e) => (
              <li key={e.message}>
                <span>{e.message}</span>
                <strong>{e.count}</strong>
              </li>
            ))}
          </ul>
        </div>
      ) : null}
    </section>
  )
}
