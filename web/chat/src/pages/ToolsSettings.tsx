import { useEffect, useState } from 'react'
import { listTools, type ToolInfo } from '../api'

function formatToolLine(t: ToolInfo): string {
  const method = (t.method ?? '').toUpperCase() || '—'
  const path = t.path ?? '—'
  return `${t.name} · ${method} ${path}`
}

export function ToolsSettings() {
  const [tools, setTools] = useState<ToolInfo[] | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const list = await listTools()
        if (!cancelled) {
          setTools(list)
          setError(null)
        }
      } catch (err) {
        if (!cancelled) {
          setTools(null)
          setError(err instanceof Error ? err.message : String(err))
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  return (
    <div className="settings-section">
      <h1 className="settings-heading">Tools</h1>
      {error && <p className="settings-error">无法加载 Tools：{error}</p>}
      {!error && tools === null && <p className="settings-muted">加载中…</p>}
      {!error && tools !== null && tools.length === 0 && (
        <p className="settings-empty">尚未注册 Connector</p>
      )}
      {!error && tools !== null && tools.length > 0 && (
        <ul className="settings-list">
          {tools.map((t) => (
            <li key={`${t.connector_id}:${t.name}`} className="settings-list-item">
              <span>{formatToolLine(t)}</span>
              {t.require_approval && (
                <span className="settings-badge">需审批</span>
              )}
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}