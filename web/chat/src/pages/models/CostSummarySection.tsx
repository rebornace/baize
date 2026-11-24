import { useEffect, useState } from 'react'
import { Button, Input } from '../../components/ui'
import { MODELS } from '../../strings'
import {
  getCostSummary,
  putCostBudget,
  type CostSummary,
} from '../../api/settings/models'

export function CostSummarySection({ readOnly }: { readOnly: boolean }) {
  const [summary, setSummary] = useState<CostSummary | null>(null)
  const [budget, setBudget] = useState('0')
  const [monthBudget, setMonthBudget] = useState('0')
  const [enforce, setEnforce] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = async () => {
    try {
      const body = await getCostSummary(7)
      setSummary(body)
      setBudget(String(body.budget_usd_per_day))
      setMonthBudget(String(body.budget_usd_per_month))
      setEnforce(body.budget_enforce)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  useEffect(() => {
    void load()
  }, [])

  const save = async () => {
    const n = Number.parseFloat(budget)
    const m = Number.parseFloat(monthBudget)
    if (!Number.isFinite(n) || n < 0 || !Number.isFinite(m) || m < 0) {
      setError(MODELS.errInvalidRequest)
      return
    }
    setBusy(true)
    try {
      await putCostBudget(n, m, enforce)
      await load()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setBusy(false)
    }
  }

  const nearBudget = summary
    ? (summary.budget_usd_per_day > 0 &&
        summary.today_total_usd / summary.budget_usd_per_day >= 0.8 &&
        !summary.over_budget) ||
      (summary.budget_usd_per_month > 0 &&
        summary.month_total_usd / summary.budget_usd_per_month >= 0.8 &&
        !summary.month_over_budget)
    : false

  return (
    <section className="settings-model-prices" data-testid="cost-summary">
      <h2 className="settings-subheading">{MODELS.costTitle}</h2>
      <p className="settings-muted">{MODELS.costHint}</p>
      {error ? <p className="settings-error">{error}</p> : null}
      {summary ? (
        <>
          <p>
            {MODELS.costToday(summary.today_total_usd, summary.currency)}
            {summary.over_budget ? (
              <span className="settings-error"> · {MODELS.costOverBudget}</span>
            ) : null}
          </p>
          <p>
            {MODELS.costMonth(summary.month_total_usd, summary.currency)}
            {summary.month_over_budget ? (
              <span className="settings-error"> · {MODELS.costMonthOverBudget}</span>
            ) : null}
            {nearBudget ? (
              <span className="settings-warn"> · {MODELS.costNearBudget}</span>
            ) : null}
          </p>
          {summary.days.some((d) => d.runs > 0) ? (
            <ul className="settings-muted">
              {summary.days.map((d) => (
                <li key={d.date}>
                  {d.date} · {summary.currency} {d.total_usd.toFixed(6)} · {d.runs}
                </li>
              ))}
            </ul>
          ) : (
            <p className="settings-muted">{MODELS.costNoData}</p>
          )}
          {!readOnly ? (
            <div className="settings-form-actions">
              <label className="settings-muted">
                {MODELS.costBudgetLabel}
                <Input
                  type="number"
                  min={0}
                  value={budget}
                  disabled={busy}
                  onChange={(e) => setBudget(e.target.value)}
                />
              </label>
              <label className="settings-muted">
                {MODELS.costMonthLabel}
                <Input
                  type="number"
                  min={0}
                  value={monthBudget}
                  disabled={busy}
                  onChange={(e) => setMonthBudget(e.target.value)}
                />
              </label>
              <label className="settings-muted">
                <input
                  type="checkbox"
                  checked={enforce}
                  disabled={busy}
                  onChange={(e) => setEnforce(e.target.checked)}
                />{' '}
                {MODELS.costEnforceLabel}
              </label>
              <Button type="button" size="sm" variant="secondary" disabled={busy} onClick={() => void save()}>
                {MODELS.costSaveBudget}
              </Button>
            </div>
          ) : null}
          <p className="settings-muted">{MODELS.costWebhookHint}</p>
        </>
      ) : null}
    </section>
  )
}
