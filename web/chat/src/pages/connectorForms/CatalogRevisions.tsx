import { useEffect, useState } from 'react'
import { listCatalogRevisions, type CatalogRevisionRow } from '../../api/connectors'
import { CONNECTORS } from '../../strings'

export function CatalogRevisions({ connectorId }: { connectorId: string }) {
  const [rows, setRows] = useState<CatalogRevisionRow[] | null>(null)
  const [err, setErr] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setRows(null)
    setErr(null)
    void listCatalogRevisions(connectorId)
      .then((list) => {
        if (!cancelled) setRows(list)
      })
      .catch((e: unknown) => {
        if (!cancelled) setErr(e instanceof Error ? e.message : String(e))
      })
    return () => {
      cancelled = true
    }
  }, [connectorId])

  if (!connectorId) return null

  return (
    <details className="catalog-revisions">
      <summary>{CONNECTORS.catalogRevisionsTitle}</summary>
      {err && <p className="muted">{err}</p>}
      {rows === null && !err && <p className="muted">{CONNECTORS.catalogRevisionsLoading}</p>}
      {rows && rows.length === 0 && <p className="muted">{CONNECTORS.catalogRevisionsEmpty}</p>}
      {rows && rows.length > 0 && (
        <ul className="catalog-revisions-list">
          {rows.map((r) => {
            const diff = r.diff as { added?: string[]; removed?: string[]; changed?: { name?: string }[] } | undefined
            const added = diff?.added?.length ?? 0
            const removed = diff?.removed?.length ?? 0
            const changed = diff?.changed?.length ?? 0
            return (
              <li key={r.revision}>
                <strong>rev {r.revision}</strong>
                {r.at ? ` · ${new Date(r.at).toLocaleString()}` : ''}
                <span className="muted">
                  {' '}
                  — {CONNECTORS.catalogDiffLine(added, removed, changed)}
                </span>
              </li>
            )
          })}
        </ul>
      )}
    </details>
  )
}
