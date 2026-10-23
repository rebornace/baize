import { useCallback, useEffect, useMemo, useState } from 'react'
import { Activity } from 'lucide-react'
import { Link } from 'react-router-dom'
import { getMetricsText, getRecentLogs } from '../api/metrics'
import { Button, EmptyState, PageHeader, ToastRegion, useToast } from '../components/ui'
import { dateTimeLocale } from '../conversationUpdatedLabel'
import { useLocale } from '../locale/LocaleContext'
import { METRICS, friendlyError } from '../strings'
import {
  buildMetricsOverview,
  formatDurationSeconds,
  type MetricsOverview,
} from './metricsParse'

type Tab = 'overview' | 'logs'

function statusLabel(status: string): string {
  return METRICS.runStatus(status)
}

function overviewCopyText(o: MetricsOverview): string {
  const parts = [
    METRICS.statRuns(o.runsTotal),
    ...o.runsByStatus.map((r) => `${statusLabel(r.status)} ${Math.round(r.count)}`),
    METRICS.statTools(o.toolOk + o.toolError),
    METRICS.statToolOk(o.toolOk),
    METRICS.statToolError(o.toolError),
  ]
  if (o.avgRunSeconds != null) {
    parts.push(METRICS.statAvgDuration(formatDurationSeconds(o.avgRunSeconds)))
  }
  return parts.join('\n')
}

export function MetricsSettings() {
  const { locale } = useLocale()
  const [tab, setTab] = useState<Tab>('overview')
  const [metricsText, setMetricsText] = useState('')
  const [logText, setLogText] = useState('')
  const [busy, setBusy] = useState(false)
  const [loadedAt, setLoadedAt] = useState<string | null>(null)
  const { toasts, push, dismiss } = useToast()

  const overview = useMemo(
    () => (metricsText ? buildMetricsOverview(metricsText) : null),
    [metricsText],
  )

  const formatLoadedAt = useCallback(
    () => new Date().toLocaleString(dateTimeLocale(locale)),
    [locale],
  )

  const loadMetrics = useCallback(async () => {
    setBusy(true)
    try {
      const body = await getMetricsText()
      setMetricsText(body)
      setLoadedAt(formatLoadedAt())
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }, [push, formatLoadedAt])

  const loadLogs = useCallback(async () => {
    setBusy(true)
    try {
      const body = await getRecentLogs(200)
      setLogText(body.lines.join('\n'))
      setLoadedAt(formatLoadedAt())
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }, [push, formatLoadedAt])

  const load = useCallback(async () => {
    if (tab === 'overview') await loadMetrics()
    else await loadLogs()
  }, [tab, loadMetrics, loadLogs])

  useEffect(() => {
    void load()
  }, [load])

  async function copyAll() {
    const text =
      tab === 'overview'
        ? overview
          ? overviewCopyText(overview)
          : metricsText
        : logText
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      push({ tone: 'success', title: METRICS.copied })
    } catch {
      push({ tone: 'error', title: METRICS.copyFailed })
    }
  }

  async function copyRaw() {
    if (!metricsText) return
    try {
      await navigator.clipboard.writeText(metricsText)
      push({ tone: 'success', title: METRICS.copied })
    } catch {
      push({ tone: 'error', title: METRICS.copyFailed })
    }
  }

  const canCopy =
    tab === 'overview' ? Boolean(overview || metricsText) : Boolean(logText)

  return (
    <div className="settings-panel settings-metrics">
      <PageHeader
        title={METRICS.title}
        description={METRICS.description}
        actions={
          <>
            <Button variant="secondary" onClick={() => void copyAll()} disabled={!canCopy || busy}>
              {METRICS.copy}
            </Button>
            <Button onClick={() => void load()} disabled={busy}>
              {busy ? METRICS.refreshing : METRICS.refresh}
            </Button>
          </>
        }
      />

      <div className="locale-choice runtime-tabs" role="tablist" aria-label={METRICS.title}>
        {(
          [
            { id: 'overview' as const, label: METRICS.tabOverview },
            { id: 'logs' as const, label: METRICS.tabLogs },
          ] as const
        ).map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            className="locale-choice-option"
            aria-selected={tab === t.id}
            data-testid={`metrics-tab-${t.id === 'overview' ? 'metrics' : 'logs'}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      <p className="settings-muted settings-metrics-meta">
        <span>{tab === 'overview' ? METRICS.overviewHint : METRICS.logsHint}</span>
        {loadedAt ? <span>{METRICS.loadedAt(loadedAt)}</span> : null}
        <Link to="/settings/runtime" data-testid="metrics-link-runtime-obs">
          {METRICS.linkRuntimeObs}
        </Link>
      </p>

      {tab === 'overview' ? (
        <div data-testid="metrics-body">
          {busy && !metricsText ? (
            <p className="settings-muted">{METRICS.refreshing}</p>
          ) : overview == null ||
            (overview.runsTotal === 0 &&
              overview.toolOk + overview.toolError === 0 &&
              overview.mcpOk + overview.mcpError === 0) ? (
            <EmptyState
              icon={<Activity size={28} aria-hidden="true" />}
              title={METRICS.empty}
              description={METRICS.emptyDesc}
            />
          ) : (
            <div className="settings-metrics-overview">
              <section className="settings-metrics-section" aria-labelledby="metrics-runs-heading">
                <h2 id="metrics-runs-heading" className="settings-subheading">
                  {METRICS.sectionRuns}
                </h2>
                <ul className="settings-metrics-rows">
                  <li className="settings-metrics-row">
                    <span>{METRICS.rowRunsTotal}</span>
                    <strong>{Math.round(overview.runsTotal)}</strong>
                  </li>
                  {overview.runsByStatus.map((r) => (
                    <li key={r.status} className="settings-metrics-row">
                      <span>{statusLabel(r.status)}</span>
                      <strong>{Math.round(r.count)}</strong>
                    </li>
                  ))}
                  <li className="settings-metrics-row">
                    <span>{METRICS.rowAvgDuration}</span>
                    <strong>
                      {overview.avgRunSeconds == null
                        ? '—'
                        : formatDurationSeconds(overview.avgRunSeconds)}
                    </strong>
                  </li>
                </ul>
              </section>

              <section className="settings-metrics-section" aria-labelledby="metrics-tools-heading">
                <h2 id="metrics-tools-heading" className="settings-subheading">
                  {METRICS.sectionTools}
                </h2>
                <ul className="settings-metrics-rows">
                  <li className="settings-metrics-row">
                    <span>{METRICS.rowToolOk}</span>
                    <strong>{Math.round(overview.toolOk)}</strong>
                  </li>
                  <li className="settings-metrics-row">
                    <span>{METRICS.rowToolError}</span>
                    <strong>{Math.round(overview.toolError)}</strong>
                  </li>
                  <li className="settings-metrics-row">
                    <span>{METRICS.rowMcpOk}</span>
                    <strong>{Math.round(overview.mcpOk)}</strong>
                  </li>
                  <li className="settings-metrics-row">
                    <span>{METRICS.rowMcpError}</span>
                    <strong>{Math.round(overview.mcpError)}</strong>
                  </li>
                </ul>
              </section>
            </div>
          )}

          {metricsText ? (
            <details className="settings-metrics-raw">
              <summary>{METRICS.rawTitle}</summary>
              <div className="settings-metrics-raw-body">
                <p className="settings-muted">{METRICS.rawHint}</p>
                <div className="settings-form-actions">
                  <Button type="button" size="sm" variant="secondary" onClick={() => void copyRaw()}>
                    {METRICS.copyRaw}
                  </Button>
                </div>
                <pre className="settings-log-viewer" data-testid="metrics-raw-body">
                  {metricsText}
                </pre>
              </div>
            </details>
          ) : null}
        </div>
      ) : (
        <>
          {!logText && !busy ? (
            <EmptyState
              icon={<Activity size={28} aria-hidden="true" />}
              title={METRICS.emptyLogs}
              description={METRICS.emptyLogsDesc}
            />
          ) : (
            <pre className="settings-log-viewer" data-testid="logs-body">
              {logText || METRICS.refreshing}
            </pre>
          )}
        </>
      )}
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </div>
  )
}
