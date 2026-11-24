import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { Wallet } from 'lucide-react'
import { getCostStatus, type CostStatus } from '../../api/settings/models'
import { CHAT } from '../../strings'

const POLL_MS = 60_000

/**
 * Sidebar budget indicator for admins. Fetches a compact status on mount and
 * every minute; hidden when no budget is configured or the API is unavailable.
 */
export function BudgetChip({ enabled }: { enabled: boolean }) {
  const [status, setStatus] = useState<CostStatus | null>(null)

  useEffect(() => {
    if (!enabled) return
    let cancelled = false
    const load = async () => {
      try {
        const next = await getCostStatus()
        if (!cancelled) setStatus(next)
      } catch {
        // Budget info is optional; hide it when the account cannot read it.
      }
    }
    void load()
    const id = window.setInterval(() => void load(), POLL_MS)
    return () => {
      cancelled = true
      window.clearInterval(id)
    }
  }, [enabled])

  if (!enabled || !status || !status.alert_level) return null

  const monthly = status.alert_scope === 'month'
  const scope = monthly ? CHAT.budgetMonth : CHAT.budgetToday
  const ratio = monthly ? status.month_ratio : status.day_ratio
  const total = monthly ? status.month_total_usd : status.today_total_usd
  const budget = monthly ? status.budget_usd_per_month : status.budget_usd_per_day
  const text =
    status.alert_level === 'exceeded'
      ? CHAT.budgetExceeded(scope, total, budget)
      : CHAT.budgetWarning(scope, Math.round(ratio * 100))

  return (
    <Link
      to="/settings/models"
      className={`budget-chip ${status.alert_level}`}
      data-testid="budget-chip"
    >
      <Wallet size={14} aria-hidden="true" />
      <span>{text}</span>
    </Link>
  )
}
