import { useEffect, useState } from 'react'
import { getBudgetAlertHistory, type BudgetAlertEntry } from '../api/metrics'
import { METRICS } from '../strings'

function targetLabel(entry: BudgetAlertEntry): string {
  if (!entry.target) return METRICS.budgetHistoryGlobal
  return entry.target_id ? `${entry.target}:${entry.target_id}` : entry.target
}

function periodLabel(period: string): string {
  return period === 'month' ? METRICS.budgetHistoryMonth : METRICS.budgetHistoryDay
}

function levelLabel(level: string): string {
  return level === 'exceeded' ? METRICS.budgetHistoryExceeded : METRICS.budgetHistoryWarning
}

export function BudgetAlertHistoryCard() {
  const [items, setItems] = useState<BudgetAlertEntry[] | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let cancelled = false
    void getBudgetAlertHistory()
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

  if (failed) return <p className="settings-muted">{METRICS.budgetHistoryLoadFailed}</p>
  if (!items) return <p className="settings-muted">{METRICS.refreshing}</p>

  return (
    <section className="run-overview" data-testid="budget-alert-history">
      <div className="run-overview-head">
        <h2 className="settings-subheading">{METRICS.budgetHistoryTitle}</h2>
      </div>
      {items.length === 0 ? (
        <p className="settings-muted">{METRICS.budgetHistoryEmpty}</p>
      ) : (
        <ul className="settings-muted">
          {items.map((entry, index) => (
            <li key={`${entry.at}-${index}`}>
              {METRICS.budgetHistoryEntry(
                new Date(entry.at).toLocaleString(),
                targetLabel(entry),
                periodLabel(entry.period),
                levelLabel(entry.level),
                entry.total_usd,
                entry.budget_usd,
                entry.ratio,
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  )
}
