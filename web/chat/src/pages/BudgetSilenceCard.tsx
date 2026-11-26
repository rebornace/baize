import { useCallback, useEffect, useState } from 'react'
import {
  getBudgetAlertSilence,
  setBudgetAlertSilence,
  type BudgetAlertSilence,
} from '../api/metrics'
import { METRICS } from '../strings'

const HOUR_MS = 60 * 60 * 1000

export function BudgetSilenceCard() {
  const [state, setState] = useState<BudgetAlertSilence | null>(null)
  const [failed, setFailed] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      setState(await getBudgetAlertSilence())
    } catch {
      setFailed(true)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const apply = useCallback(async (hours: number) => {
    setBusy(true)
    try {
      const until = new Date(Date.now() + hours * HOUR_MS).toISOString()
      setState(await setBudgetAlertSilence(until))
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }, [])

  const clear = useCallback(async () => {
    setBusy(true)
    try {
      setState(await setBudgetAlertSilence(''))
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }, [])

  if (failed) return <p className="settings-muted">{METRICS.silenceFailed}</p>
  if (!state) return <p className="settings-muted">{METRICS.refreshing}</p>

  return (
    <section className="run-overview" data-testid="budget-silence">
      <div className="run-overview-head">
        <h2 className="settings-subheading">{METRICS.silenceTitle}</h2>
      </div>
      {state.active && state.until ? (
        <>
          <p className="settings-muted">
            {METRICS.silenceActive(new Date(state.until).toLocaleString())}
          </p>
          <div className="settings-form-actions">
            <button type="button" className="btn secondary sm" disabled={busy} onClick={() => void clear()}>
              {METRICS.silenceClear}
            </button>
          </div>
        </>
      ) : (
        <>
          <p className="settings-muted">{METRICS.silenceInactive}</p>
          <div className="settings-form-actions">
            <button type="button" className="btn secondary sm" disabled={busy} onClick={() => void apply(1)}>
              {METRICS.silenceHour}
            </button>
            <button type="button" className="btn secondary sm" disabled={busy} onClick={() => void apply(24)}>
              {METRICS.silenceDay}
            </button>
            <button type="button" className="btn secondary sm" disabled={busy} onClick={() => void apply(24 * 7)}>
              {METRICS.silenceWeek}
            </button>
          </div>
        </>
      )}
    </section>
  )
}
