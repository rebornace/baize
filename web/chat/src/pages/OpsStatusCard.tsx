import { useEffect, useState } from 'react'
import { getOpsStatus, type OpsStatus } from '../api/metrics'
import { METRICS } from '../strings'

function daysLabel(days: number): string {
  return days > 0 ? `${days}d` : METRICS.opsForever
}

export function OpsStatusCard() {
  const [data, setData] = useState<OpsStatus | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    void getOpsStatus()
      .then((next) => {
        if (!cancelled) setData(next)
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (failed) return <p className="settings-muted">{METRICS.opsLoadFailed}</p>
  if (!data) return <p className="settings-muted">{METRICS.refreshing}</p>

  const gate =
    data.eval_gate.last_pass === null
      ? METRICS.opsGateNone
      : data.eval_gate.last_pass
        ? METRICS.opsGatePass
        : METRICS.opsGateFail
  const alerts = [data.budget.day_alert, data.budget.month_alert].filter(Boolean)
  const budget = alerts.length > 0 ? alerts.join(' / ') : METRICS.opsNone

  return (
    <section className="run-overview" data-testid="ops-status">
      <div className="run-overview-head">
        <h2 className="settings-subheading">{METRICS.opsTitle}</h2>
      </div>
      <div className="run-overview-grid">
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.opsActive}</span>
          <strong>{data.runs.active}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.opsQueued}</span>
          <strong>{data.runs.queued}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.opsWaiting}</span>
          <strong>{data.runs.waiting_human}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.opsFailed24}</span>
          <strong>{data.runs.failed_24h}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.opsWebhook}</span>
          <strong>{data.webhook_configured ? METRICS.opsConfigured : METRICS.opsNotConfigured}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.opsBudget}</span>
          <strong>{budget}</strong>
        </div>
        <div className="run-overview-stat">
          <span className="settings-muted">{METRICS.opsEvalGate}</span>
          <strong>{gate}</strong>
        </div>
      </div>
      <p className="settings-muted">
        {METRICS.opsRetention(
          daysLabel(data.retention.attachment_days),
          daysLabel(data.retention.run_days),
          daysLabel(data.retention.message_days),
        )}
        {data.budget.scoped_alerts > 0
          ? ` · ${METRICS.opsScopedAlerts(data.budget.scoped_alerts)}`
          : ''}
      </p>
    </section>
  )
}
