import { useEffect, useState } from 'react'
import { getCostAnomalies, type CostAnomalyEntry } from '../api/metrics'
import { METRICS } from '../strings'

function scopeLabel(entry: CostAnomalyEntry): string {
  if (entry.scope === 'conversation') {
    return entry.conversation_id
      ? `${METRICS.anomalyConversation}:${entry.conversation_id}`
      : METRICS.anomalyConversation
  }
  return METRICS.anomalyGlobal
}

export function CostAnomalyCard() {
  const [items, setItems] = useState<CostAnomalyEntry[] | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    void getCostAnomalies(50)
      .then((body) => {
        if (!cancelled) setItems(body.items ?? [])
      })
      .catch(() => {
        if (!cancelled) setFailed(true)
      })
    return () => {
      cancelled = true
    }
  }, [])

  if (failed) return <p className="settings-muted">{METRICS.anomalyLoadFailed}</p>
  if (!items) return <p className="settings-muted">{METRICS.refreshing}</p>

  return (
    <section className="run-overview" data-testid="cost-anomaly-history">
      <div className="run-overview-head">
        <h2 className="settings-subheading">{METRICS.anomalyTitle}</h2>
      </div>
      {items.length === 0 ? (
        <p className="settings-muted">{METRICS.anomalyEmpty}</p>
      ) : (
        <ul className="settings-muted">
          {items.map((entry, index) => (
            <li key={`${entry.at}-${index}`}>
              {METRICS.anomalyEntry(
                new Date(entry.at).toLocaleString(),
                scopeLabel(entry),
                entry.today_usd,
                entry.baseline_usd,
                entry.ratio,
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
