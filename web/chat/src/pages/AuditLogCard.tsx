import { useCallback, useEffect, useState } from 'react'
import {
  exportAuditCSV,
  getAuditEvents,
  getAuditRetention,
  putAuditRetention,
  type AuditEvent,
} from '../api/metrics'
import { Button } from '../components/ui'
import { METRICS } from '../strings'

function dateBounds(from: string, to: string): { from?: string; to?: string } {
  const bounds: { from?: string; to?: string } = {}
  if (from) bounds.from = new Date(`${from}T00:00:00`).toISOString()
  if (to) bounds.to = new Date(`${to}T23:59:59`).toISOString()
  return bounds
}

export function AuditLogCard() {
  const [items, setItems] = useState<AuditEvent[] | null>(null)
  const [failed, setFailed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [query, setQuery] = useState('')
  const [operatorID, setOperatorID] = useState('')
  const [from, setFrom] = useState('')
  const [to, setTo] = useState('')
  const [limit, setLimit] = useState(50)
  const [retentionRows, setRetentionRows] = useState(5000)
  const [retentionBusy, setRetentionBusy] = useState(false)
  const [retentionSaved, setRetentionSaved] = useState(false)

  const load = useCallback(async () => {
    setBusy(true)
    setFailed(false)
    try {
      const body = await getAuditEvents(limit, {
        q: query.trim() || undefined,
        operator_id: operatorID.trim() || undefined,
        ...dateBounds(from, to),
      })
      setItems(body.items ?? [])
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }, [limit, query, operatorID, from, to])

  useEffect(() => {
    void load()
    void getAuditRetention()
      .then((r) => setRetentionRows(r.rows))
      .catch(() => undefined)
  }, [])

  const saveRetention = useCallback(async () => {
    setRetentionBusy(true)
    setRetentionSaved(false)
    try {
      const r = await putAuditRetention(retentionRows)
      setRetentionRows(r.rows)
      setRetentionSaved(true)
      await load()
    } catch {
      setFailed(true)
    } finally {
      setRetentionBusy(false)
    }
  }, [retentionRows, load])

  async function download() {
    setBusy(true)
    setFailed(false)
    try {
      await exportAuditCSV({
        q: query.trim() || undefined,
        operator_id: operatorID.trim() || undefined,
        ...dateBounds(from, to),
      })
    } catch {
      setFailed(true)
    } finally {
      setBusy(false)
    }
  }

  return (
    <section className="run-overview" data-testid="audit-log">
      <div className="run-overview-head">
        <h2 className="settings-subheading">{METRICS.auditTitle}</h2>
      </div>
      <div className="audit-filters">
        <input
          className="settings-input"
          type="search"
          placeholder={METRICS.auditFilterQuery}
          aria-label={METRICS.auditFilterQuery}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <input
          className="settings-input"
          type="search"
          placeholder={METRICS.auditFilterOperator}
          aria-label={METRICS.auditFilterOperator}
          value={operatorID}
          onChange={(e) => setOperatorID(e.target.value)}
        />
        <label className="audit-filter-date">
          <span className="settings-muted">{METRICS.auditFrom}</span>
          <input
            className="settings-input"
            type="date"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
        </label>
        <label className="audit-filter-date">
          <span className="settings-muted">{METRICS.auditTo}</span>
          <input
            className="settings-input"
            type="date"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          />
        </label>
        <select
          className="settings-input audit-limit"
          aria-label={METRICS.auditLimit}
          value={limit}
          onChange={(e) => setLimit(Number(e.target.value))}
        >
          <option value={50}>{METRICS.auditLimitOption(50)}</option>
          <option value={200}>{METRICS.auditLimitOption(200)}</option>
        </select>
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => void load()}>
          {METRICS.auditApply}
        </Button>
        <Button size="sm" variant="secondary" disabled={busy} onClick={() => void download()}>
          {METRICS.auditExport}
        </Button>
        <select
          className="settings-input audit-limit"
          aria-label={METRICS.auditRetentionLabel}
          value={retentionRows}
          disabled={retentionBusy}
          onChange={(e) => {
            setRetentionRows(Number(e.target.value))
            setRetentionSaved(false)
          }}
        >
          {[500, 1000, 2000, 5000].map((rows) => (
            <option key={rows} value={rows}>
              {METRICS.auditRetentionOption(rows)}
            </option>
          ))}
        </select>
        <Button
          size="sm"
          variant="secondary"
          disabled={retentionBusy}
          onClick={() => void saveRetention()}
        >
          {METRICS.auditRetentionSave}
        </Button>
        {retentionSaved ? (
          <span className="settings-muted">{METRICS.auditRetentionSaved}</span>
        ) : null}
      </div>
      {failed ? <p className="settings-muted">{METRICS.auditLoadFailed}</p> : null}
      {!failed && !items ? <p className="settings-muted">{METRICS.refreshing}</p> : null}
      {!failed && items && items.length === 0 ? (
        <p className="settings-muted">{METRICS.auditEmpty}</p>
      ) : null}
      {!failed && items && items.length > 0 ? (
        <ul className="settings-muted">
          {items.map((entry) => (
            <li key={entry.id} title={entry.target}>
              {METRICS.auditEntry(
                new Date(entry.created_at).toLocaleString(),
                entry.operator_id || METRICS.auditAnonymous,
                entry.action,
                entry.status,
              )}
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  )
}
