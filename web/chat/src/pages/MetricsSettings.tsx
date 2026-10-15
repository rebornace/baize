import { useCallback, useEffect, useState } from 'react'
import { getMetricsText, getRecentLogs } from '../api/metrics'
import { Button, PageHeader, ToastRegion, useToast } from '../components/ui'
import { METRICS, friendlyError } from '../strings'

type Tab = 'metrics' | 'logs'

export function MetricsSettings() {
  const [tab, setTab] = useState<Tab>('metrics')
  const [metricsText, setMetricsText] = useState('')
  const [logText, setLogText] = useState('')
  const [busy, setBusy] = useState(false)
  const [loadedAt, setLoadedAt] = useState<string | null>(null)
  const { toasts, push, dismiss } = useToast()

  const loadMetrics = useCallback(async () => {
    setBusy(true)
    try {
      const body = await getMetricsText()
      setMetricsText(body)
      setLoadedAt(new Date().toLocaleString())
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }, [push])

  const loadLogs = useCallback(async () => {
    setBusy(true)
    try {
      const body = await getRecentLogs(200)
      setLogText(body.lines.join('\n'))
      setLoadedAt(new Date().toLocaleString())
    } catch (e) {
      const f = friendlyError(e)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }, [push])

  const load = useCallback(async () => {
    if (tab === 'metrics') await loadMetrics()
    else await loadLogs()
  }, [tab, loadMetrics, loadLogs])

  useEffect(() => {
    void load()
  }, [load])

  async function copyAll() {
    const text = tab === 'metrics' ? metricsText : logText
    if (!text) return
    try {
      await navigator.clipboard.writeText(text)
      push({ tone: 'success', title: METRICS.copied })
    } catch {
      push({ tone: 'error', title: METRICS.copyFailed })
    }
  }

  const activeText = tab === 'metrics' ? metricsText : logText
  const emptyLabel = tab === 'metrics' ? METRICS.empty : METRICS.emptyLogs

  return (
    <div className="settings-panel">
      <PageHeader
        title={METRICS.title}
        description={METRICS.description}
        actions={
          <>
            <Button variant="secondary" onClick={() => void copyAll()} disabled={!activeText || busy}>
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
            { id: 'metrics' as const, label: METRICS.tabMetrics },
            { id: 'logs' as const, label: METRICS.tabLogs },
          ] as const
        ).map((t) => (
          <button
            key={t.id}
            type="button"
            role="tab"
            className={`locale-choice-btn${tab === t.id ? ' active' : ''}`}
            aria-selected={tab === t.id}
            data-testid={`metrics-tab-${t.id}`}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>

      <p className="settings-muted">
        {tab === 'metrics' ? (
          <>
            {METRICS.endpointHint}{' '}
            <code className="settings-code-sample" style={{ display: 'inline', padding: '0.1rem 0.35rem' }}>
              GET /metrics
            </code>
          </>
        ) : (
          <>
            {METRICS.logsHint}{' '}
            <code className="settings-code-sample" style={{ display: 'inline', padding: '0.1rem 0.35rem' }}>
              GET /v0/settings/logs
            </code>
          </>
        )}
        {loadedAt ? ` · ${METRICS.loadedAt(loadedAt)}` : null}
      </p>
      <pre
        className="settings-code-sample"
        data-testid={tab === 'metrics' ? 'metrics-body' : 'logs-body'}
        style={{ maxHeight: '70vh', overflow: 'auto' }}
      >
        {activeText || (busy ? METRICS.refreshing : emptyLabel)}
      </pre>
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </div>
  )
}
