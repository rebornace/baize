import { useEffect, useState } from 'react'
import { listTools, patchToolRequireLogin, type ToolInfo } from '../api'

function formatToolLine(t: ToolInfo): string {
  const method = (t.method ?? '').toUpperCase() || '—'
  const path = t.path ?? '—'
  return `${t.name} · ${method} ${path}`
}

export function ToolsSettings() {
  const [tools, setTools] = useState<ToolInfo[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [toggling, setToggling] = useState<string | null>(null)

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

  const onRequireLoginChange = async (name: string, requireLogin: boolean) => {
    setToggling(name)
    try {
      const updated = await patchToolRequireLogin(name, requireLogin)
      setTools((prev) =>
        prev == null
          ? prev
          : prev.map((t) => (t.name === name ? { ...t, ...updated } : t)),
      )
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setToggling(null)
    }
  }

  const loadFailed = tools === null && error !== null

  return (
    <div className="settings-section">
      <h1 className="settings-heading">Tools</h1>
      {loadFailed && <p className="settings-error">无法加载 Tools：{error}</p>}
      {!loadFailed && error && <p className="settings-error">{error}</p>}
      {tools === null && !loadFailed && <p className="settings-muted">加载中…</p>}
      {tools !== null && tools.length === 0 && (
        <p className="settings-empty">尚未注册 Connector</p>
      )}
      {tools !== null && tools.length > 0 && (
        <ul className="settings-list">
          {tools.map((t) => (
            <li key={`${t.connector_id}:${t.name}`} className="settings-list-item">
              <span className="settings-tool-line">{formatToolLine(t)}</span>
              <span className="settings-tool-actions">
                {t.require_approval && (
                  <span className="settings-badge">需审批</span>
                )}
                <label className="settings-login-toggle">
                  <input
                    type="checkbox"
                    checked={Boolean(t.require_login)}
                    disabled={toggling !== null}
                    onChange={(e) => {
                      void onRequireLoginChange(t.name, e.target.checked)
                    }}
                  />
                  需要登录
                </label>
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
