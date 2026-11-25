import { useEffect, useState } from 'react'
import { Button, Input } from '../../components/ui'
import { MODELS } from '../../strings'
import {
  downloadCostExport,
  getAgentCostBudgets,
  getCostBreakdown,
  getCostSummary,
  getModelCostBudgets,
  putAgentCostBudget,
  putCostBudget,
  putModelCostBudget,
  type AgentCostBudget,
  type CostBreakdown,
  type CostSummary,
  type ModelCostBudget,
} from '../../api/settings/models'

type AgentDraft = { day: string; month: string; warn: string; exceed: string }

const COST_TREND_DAYS = 30

export function CostSummarySection({ readOnly }: { readOnly: boolean }) {
  const [summary, setSummary] = useState<CostSummary | null>(null)
  const [breakdown, setBreakdown] = useState<CostBreakdown | null>(null)
  const [budget, setBudget] = useState('0')
  const [monthBudget, setMonthBudget] = useState('0')
  const [warnRatio, setWarnRatio] = useState('80')
  const [exceededRatio, setExceededRatio] = useState('100')
  const [enforce, setEnforce] = useState(false)
  const [busy, setBusy] = useState(false)
  const [exportBusy, setExportBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [agents, setAgents] = useState<AgentCostBudget[] | null>(null)
  const [agentDrafts, setAgentDrafts] = useState<Record<string, AgentDraft>>({})
  const [agentBusy, setAgentBusy] = useState<string | null>(null)
  const [models, setModels] = useState<ModelCostBudget[] | null>(null)
  const [modelDrafts, setModelDrafts] = useState<Record<string, AgentDraft>>({})
  const [modelBusy, setModelBusy] = useState<string | null>(null)

  const load = async () => {
    try {
      const body = await getCostSummary(COST_TREND_DAYS)
      setSummary(body)
      setBudget(String(body.budget_usd_per_day))
      setMonthBudget(String(body.budget_usd_per_month))
      setWarnRatio(String(Math.round((body.warning_ratio ?? 0.8) * 100)))
      setExceededRatio(String(Math.round((body.exceeded_ratio ?? 1) * 100)))
      setEnforce(body.budget_enforce)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }

  const loadAgents = async () => {
    try {
      const body = await getAgentCostBudgets()
      setAgents(body.items)
      const drafts: Record<string, AgentDraft> = {}
      for (const item of body.items) {
        drafts[item.agent_id] = {
          day: String(item.budget_usd_per_day),
          month: String(item.budget_usd_per_month),
          warn: String(Math.round(item.warning_ratio * 100)),
          exceed: String(Math.round(item.exceeded_ratio * 100)),
        }
      }
      setAgentDrafts(drafts)
    } catch {
      setAgents(null)
    }
  }

  const loadModels = async () => {
    try {
      const body = await getModelCostBudgets()
      setModels(body.items)
      const drafts: Record<string, AgentDraft> = {}
      for (const item of body.items) {
        drafts[item.model_profile_id] = {
          day: String(item.budget_usd_per_day),
          month: String(item.budget_usd_per_month),
          warn: String(Math.round(item.warning_ratio * 100)),
          exceed: String(Math.round(item.exceeded_ratio * 100)),
        }
      }
      setModelDrafts(drafts)
    } catch {
      setModels(null)
    }
  }

  useEffect(() => {
    void load()
    void loadAgents()
    void loadModels()
    void getCostBreakdown(30)
      .then(setBreakdown)
      .catch(() => setBreakdown(null))
  }, [])

  const save = async () => {
    const n = Number.parseFloat(budget)
    const m = Number.parseFloat(monthBudget)
    const w = Number.parseFloat(warnRatio)
    const e = Number.parseFloat(exceededRatio)
    if (
      !Number.isFinite(n) ||
      n < 0 ||
      !Number.isFinite(m) ||
      m < 0 ||
      !Number.isFinite(w) ||
      w <= 0 ||
      w > 500 ||
      !Number.isFinite(e) ||
      e <= 0 ||
      e > 500 ||
      w >= e
    ) {
      setError(MODELS.errInvalidRequest)
      return
    }
    setBusy(true)
    try {
      await putCostBudget(n, m, enforce, w / 100, e / 100)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setBusy(false)
    }
  }

  const saveAgent = async (item: AgentCostBudget) => {
    const draft = agentDrafts[item.agent_id] ?? { day: '0', month: '0', warn: '0', exceed: '0' }
    const d = Number.parseFloat(draft.day)
    const m = Number.parseFloat(draft.month)
    const w = Number.parseFloat(draft.warn)
    const e = Number.parseFloat(draft.exceed)
    if (
      !Number.isFinite(d) || d < 0 ||
      !Number.isFinite(m) || m < 0 ||
      !Number.isFinite(w) || w < 0 || w > 500 ||
      !Number.isFinite(e) || e < 0 || e > 500 ||
      (w > 0 && e > 0 && w >= e)
    ) {
      setError(MODELS.errInvalidRequest)
      return
    }
    setAgentBusy(item.agent_id)
    try {
      const next = await putAgentCostBudget({
        agent_id: item.agent_id,
        budget_usd_per_day: d,
        budget_usd_per_month: m,
        warning_ratio: w / 100,
        exceeded_ratio: e / 100,
      })
      setAgents((prev) =>
        prev ? prev.map((a) => (a.agent_id === next.agent_id ? next : a)) : prev,
      )
      setAgentDrafts((prev) => ({
        ...prev,
        [next.agent_id]: {
          day: String(next.budget_usd_per_day),
          month: String(next.budget_usd_per_month),
          warn: String(Math.round(next.warning_ratio * 100)),
          exceed: String(Math.round(next.exceeded_ratio * 100)),
        },
      }))
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setAgentBusy(null)
    }
  }

  const saveModel = async (item: ModelCostBudget) => {
    const draft = modelDrafts[item.model_profile_id] ?? { day: '0', month: '0', warn: '0', exceed: '0' }
    const d = Number.parseFloat(draft.day)
    const m = Number.parseFloat(draft.month)
    const w = Number.parseFloat(draft.warn)
    const e = Number.parseFloat(draft.exceed)
    if (
      !Number.isFinite(d) || d < 0 ||
      !Number.isFinite(m) || m < 0 ||
      !Number.isFinite(w) || w < 0 || w > 500 ||
      !Number.isFinite(e) || e < 0 || e > 500 ||
      (w > 0 && e > 0 && w >= e)
    ) {
      setError(MODELS.errInvalidRequest)
      return
    }
    setModelBusy(item.model_profile_id)
    try {
      const next = await putModelCostBudget({
        model_profile_id: item.model_profile_id,
        budget_usd_per_day: d,
        budget_usd_per_month: m,
        warning_ratio: w / 100,
        exceeded_ratio: e / 100,
      })
      setModels((prev) =>
        prev ? prev.map((row) => (row.model_profile_id === next.model_profile_id ? next : row)) : prev,
      )
      setModelDrafts((prev) => ({
        ...prev,
        [next.model_profile_id]: {
          day: String(next.budget_usd_per_day),
          month: String(next.budget_usd_per_month),
          warn: String(Math.round(next.warning_ratio * 100)),
          exceed: String(Math.round(next.exceeded_ratio * 100)),
        },
      }))
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setModelBusy(null)
    }
  }

  const exportCsv = async () => {
    setExportBusy(true)
    try {
      const blob = await downloadCostExport(COST_TREND_DAYS)
      if (typeof URL.createObjectURL !== 'function') return
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = `baize-cost-${new Date().toISOString().slice(0, 10)}.csv`
      document.body.appendChild(link)
      link.click()
      link.remove()
      URL.revokeObjectURL(url)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setExportBusy(false)
    }
  }

  const series = summary?.days ?? []
  const seriesTotal = series.reduce((sum, d) => sum + d.total_usd, 0)
  const maxDayCost = series.reduce((max, d) => Math.max(max, d.total_usd), 0)
  const hasCostData = series.some((d) => d.runs > 0 || d.total_usd > 0)

  const nearBudget = summary
    ? (summary.budget_usd_per_day > 0 &&
        summary.today_total_usd / summary.budget_usd_per_day >= (summary.warning_ratio ?? 0.8) &&
        !summary.over_budget) ||
      (summary.budget_usd_per_month > 0 &&
        summary.month_total_usd / summary.budget_usd_per_month >=
          (summary.warning_ratio ?? 0.8) &&
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
          <p>
            {MODELS.costProjectedMonth(
              summary.projected_month_usd,
              summary.projection_day,
              summary.days_in_month,
            )}
            {summary.projected_over_budget ? (
              <span className="settings-error"> · {MODELS.costProjectedOver}</span>
            ) : null}
          </p>
          {hasCostData ? (
            <div className="cost-trend" data-testid="cost-trend">
              <div className="cost-trend-head">
                <h3 className="settings-subheading">{MODELS.costTrendTitle}</h3>
                <span className="settings-muted">{MODELS.costTrendTotal(seriesTotal, series.length)}</span>
                <Button
                  type="button"
                  size="sm"
                  variant="secondary"
                  disabled={exportBusy}
                  onClick={() => void exportCsv()}
                  data-testid="cost-export"
                >
                  {exportBusy ? MODELS.costExporting : MODELS.costExport}
                </Button>
              </div>
              <div
                className="cost-trend-bars"
                role="img"
                aria-label={MODELS.costTrendTitle}
              >
                {series.map((d) => {
                  const pct =
                    maxDayCost > 0 ? Math.max(2, Math.round((d.total_usd / maxDayCost) * 100)) : 0
                  return (
                    <div
                      key={d.date}
                      className="cost-trend-bar"
                      data-testid="cost-trend-bar"
                      title={`${d.date} · ${summary.currency} ${d.total_usd.toFixed(6)} · ${d.runs}`}
                    >
                      <span style={{ height: `${pct}%` }} />
                    </div>
                  )
                })}
              </div>
              <div className="cost-trend-axis settings-muted">
                <span>{series[0]?.date}</span>
                <span>{series[series.length - 1]?.date}</span>
              </div>
            </div>
          ) : (
            <p className="settings-muted">{MODELS.costNoData}</p>
          )}
          {breakdown && breakdown.total_usd > 0 ? (
            <div className="cost-breakdown" data-testid="cost-breakdown">
              <h3 className="settings-subheading">{MODELS.costBreakdownTitle}</h3>
              <div className="cost-breakdown-grid">
                <div>
                  <span className="settings-muted">{MODELS.costByModel}</span>
                  <ul>
                    {breakdown.by_model.map((b) => (
                      <li key={b.key || 'default'}>
                        <span>{b.name || b.key || MODELS.costDefaultModel}</span>
                        <strong>
                          ${b.cost_usd.toFixed(4)} · {Math.round(b.ratio * 100)}%
                        </strong>
                      </li>
                    ))}
                  </ul>
                </div>
                <div>
                  <span className="settings-muted">{MODELS.costByAgent}</span>
                  <ul>
                    {breakdown.by_agent.map((b) => (
                      <li key={b.key || 'default'}>
                        <span>{b.name || b.key || MODELS.costDefaultModel}</span>
                        <strong>
                          ${b.cost_usd.toFixed(4)} · {Math.round(b.ratio * 100)}%
                        </strong>
                      </li>
                    ))}
                  </ul>
                </div>
              </div>
            </div>
          ) : null}
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
                {MODELS.costWarnRatioLabel}
                <Input
                  type="number"
                  min={1}
                  max={500}
                  value={warnRatio}
                  disabled={busy}
                  onChange={(e) => setWarnRatio(e.target.value)}
                  data-testid="cost-warning-ratio"
                />
              </label>
              <label className="settings-muted">
                {MODELS.costExceededRatioLabel}
                <Input
                  type="number"
                  min={1}
                  max={500}
                  value={exceededRatio}
                  disabled={busy}
                  onChange={(e) => setExceededRatio(e.target.value)}
                  data-testid="cost-exceeded-ratio"
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
          {!readOnly && agents && agents.length > 0 ? (
            <div className="agent-budgets" data-testid="agent-budgets">
              <h3 className="settings-subheading">{MODELS.agentBudgetTitle}</h3>
              <p className="settings-muted">{MODELS.agentBudgetHint}</p>
              {agents.map((item) => {
                const draft = agentDrafts[item.agent_id] ?? { day: '0', month: '0', warn: '0', exceed: '0' }
                return (
                  <div className="agent-budget-row" key={item.agent_id}>
                    <span className="agent-budget-id">{item.agent_id}</span>
                    {item.budget_usd_per_day > 0 || item.budget_usd_per_month > 0 ? (
                      <span className="settings-muted agent-budget-spent">
                        {MODELS.agentBudgetSpent(item.spent_usd_per_day, item.spent_usd_per_month)}
                      </span>
                    ) : null}
                    <label className="settings-muted">
                      {MODELS.agentBudgetDayLabel}
                      <Input
                        type="number"
                        min={0}
                        value={draft.day}
                        disabled={agentBusy === item.agent_id}
                        onChange={(e) =>
                          setAgentDrafts((prev) => ({
                            ...prev,
                            [item.agent_id]: { ...draft, day: e.target.value },
                          }))
                        }
                      />
                    </label>
                    <label className="settings-muted">
                      {MODELS.agentBudgetMonthLabel}
                      <Input
                        type="number"
                        min={0}
                        value={draft.month}
                        disabled={agentBusy === item.agent_id}
                        onChange={(e) =>
                          setAgentDrafts((prev) => ({
                            ...prev,
                            [item.agent_id]: { ...draft, month: e.target.value },
                          }))
                        }
                      />
                    </label>
                    <label className="settings-muted">
                      {MODELS.agentBudgetWarnLabel}
                      <Input
                        type="number"
                        min={0}
                        max={500}
                        value={draft.warn}
                        disabled={agentBusy === item.agent_id}
                        onChange={(e) =>
                          setAgentDrafts((prev) => ({
                            ...prev,
                            [item.agent_id]: { ...draft, warn: e.target.value },
                          }))
                        }
                      />
                    </label>
                    <label className="settings-muted">
                      {MODELS.agentBudgetExceedLabel}
                      <Input
                        type="number"
                        min={0}
                        max={500}
                        value={draft.exceed}
                        disabled={agentBusy === item.agent_id}
                        onChange={(e) =>
                          setAgentDrafts((prev) => ({
                            ...prev,
                            [item.agent_id]: { ...draft, exceed: e.target.value },
                          }))
                        }
                      />
                    </label>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={agentBusy === item.agent_id}
                      onClick={() => void saveAgent(item)}
                    >
                      {MODELS.agentBudgetSave}
                    </Button>
                  </div>
                )
              })}
            </div>
          ) : null}
          {!readOnly && models && models.length > 0 ? (
            <div className="agent-budgets" data-testid="model-budgets">
              <h3 className="settings-subheading">{MODELS.modelBudgetTitle}</h3>
              <p className="settings-muted">{MODELS.modelBudgetHint}</p>
              {models.map((item) => {
                const draft = modelDrafts[item.model_profile_id] ?? {
                  day: '0',
                  month: '0',
                  warn: '0',
                  exceed: '0',
                }
                return (
                  <div className="agent-budget-row" key={item.model_profile_id}>
                    <span className="agent-budget-id" title={item.model}>
                      {item.name || item.model}
                    </span>
                    {item.budget_usd_per_day > 0 || item.budget_usd_per_month > 0 ? (
                      <span className="settings-muted agent-budget-spent">
                        {MODELS.modelBudgetSpent(item.spent_usd_per_day, item.spent_usd_per_month)}
                      </span>
                    ) : null}
                    <label className="settings-muted">
                      {MODELS.agentBudgetDayLabel}
                      <Input
                        type="number"
                        min={0}
                        value={draft.day}
                        disabled={modelBusy === item.model_profile_id}
                        onChange={(e) =>
                          setModelDrafts((prev) => ({
                            ...prev,
                            [item.model_profile_id]: { ...draft, day: e.target.value },
                          }))
                        }
                      />
                    </label>
                    <label className="settings-muted">
                      {MODELS.agentBudgetMonthLabel}
                      <Input
                        type="number"
                        min={0}
                        value={draft.month}
                        disabled={modelBusy === item.model_profile_id}
                        onChange={(e) =>
                          setModelDrafts((prev) => ({
                            ...prev,
                            [item.model_profile_id]: { ...draft, month: e.target.value },
                          }))
                        }
                      />
                    </label>
                    <label className="settings-muted">
                      {MODELS.agentBudgetWarnLabel}
                      <Input
                        type="number"
                        min={0}
                        max={500}
                        value={draft.warn}
                        disabled={modelBusy === item.model_profile_id}
                        onChange={(e) =>
                          setModelDrafts((prev) => ({
                            ...prev,
                            [item.model_profile_id]: { ...draft, warn: e.target.value },
                          }))
                        }
                      />
                    </label>
                    <label className="settings-muted">
                      {MODELS.agentBudgetExceedLabel}
                      <Input
                        type="number"
                        min={0}
                        max={500}
                        value={draft.exceed}
                        disabled={modelBusy === item.model_profile_id}
                        onChange={(e) =>
                          setModelDrafts((prev) => ({
                            ...prev,
                            [item.model_profile_id]: { ...draft, exceed: e.target.value },
                          }))
                        }
                      />
                    </label>
                    <Button
                      type="button"
                      size="sm"
                      variant="secondary"
                      disabled={modelBusy === item.model_profile_id}
                      onClick={() => void saveModel(item)}
                    >
                      {MODELS.agentBudgetSave}
                    </Button>
                  </div>
                )
              })}
            </div>
          ) : null}
        </>
      ) : null}
    </section>
  )
}
