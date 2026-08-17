import { useEffect, useMemo, useState } from 'react'
import {
  createConnectorTool,
  deleteConnectorTool,
  listTools,
  patchTool,
  patchToolRequireLogin,
  type ToolInfo,
} from '../api'
import { canDeleteCatalogTool } from '../toolCatalog'

function formatToolLine(t: ToolInfo): string {
  const method = (t.method ?? '').toUpperCase() || '—'
  const path = t.path ?? '—'
  return `${t.name} · ${method} ${path}`
}

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']

interface AddFormState {
  name: string
  method: string
  path: string
  description: string
  schema: string
}

const EMPTY_FORM: AddFormState = {
  name: '',
  method: 'GET',
  path: '',
  description: '',
  schema: '{}',
}

export function ToolsSettings() {
  const [tools, setTools] = useState<ToolInfo[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [toggling, setToggling] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<string | null>(null)
  const [form, setForm] = useState<AddFormState>(EMPTY_FORM)
  const [formConnectorId, setFormConnectorId] = useState<string>('')
  const [submitting, setSubmitting] = useState(false)

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

  const openConnectorIds = useMemo(() => {
    if (tools == null) return [] as string[]
    const seen = new Set<string>()
    const ordered: string[] = []
    for (const t of tools) {
      if (t.source === 'plugin') continue
      if (!t.connector_id) continue
      if (seen.has(t.connector_id)) continue
      seen.add(t.connector_id)
      ordered.push(t.connector_id)
    }
    return ordered
  }, [tools])

  useEffect(() => {
    if (openConnectorIds.length > 0 && !openConnectorIds.includes(formConnectorId)) {
      setFormConnectorId(openConnectorIds[0])
    }
    if (openConnectorIds.length === 0 && formConnectorId !== '') {
      setFormConnectorId('')
    }
  }, [openConnectorIds, formConnectorId])

  const showAddForm = openConnectorIds.length > 0

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

  const onEnabledChange = async (name: string, enabled: boolean) => {
    setToggling(name)
    try {
      const updated = await patchTool(name, { enabled })
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

  const onDelete = async (t: ToolInfo) => {
    const key = `${t.connector_id}:${t.name}`
    setDeleting(key)
    try {
      await deleteConnectorTool(t.connector_id, t.name)
      setTools((prev) =>
        prev == null ? prev : prev.filter((row) => row.name !== t.name || row.connector_id !== t.connector_id),
      )
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setDeleting(null)
    }
  }

  const onAddSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!formConnectorId) {
      setError('未选择 Connector')
      return
    }
    let schema: Record<string, unknown> = {}
    try {
      schema = form.schema.trim() === '' ? {} : (JSON.parse(form.schema) as Record<string, unknown>)
    } catch {
      setError('input_schema 不是合法 JSON')
      return
    }
    setSubmitting(true)
    try {
      const created = await createConnectorTool(formConnectorId, {
        name: form.name.trim(),
        method: form.method,
        path: form.path.trim(),
        description: form.description.trim() || undefined,
        input_schema: schema,
      })
      setTools((prev) => (prev == null ? prev : [...prev, created]))
      setForm(EMPTY_FORM)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setSubmitting(false)
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
          {tools.map((t) => {
            const key = `${t.connector_id}:${t.name}`
            const canDelete = canDeleteCatalogTool(t.source ?? '')
            const isToggling = toggling !== null
            const isDeleting = deleting === key
            return (
              <li key={key} className="settings-list-item">
                <span className="settings-tool-line">{formatToolLine(t)}</span>
                <span className="settings-tool-actions">
                  {t.require_approval && (
                    <span className="settings-badge">需审批</span>
                  )}
                  <label className="settings-login-toggle">
                    <input
                      type="checkbox"
                      checked={Boolean(t.enabled ?? true)}
                      disabled={isToggling}
                      onChange={(e) => {
                        void onEnabledChange(t.name, e.target.checked)
                      }}
                    />
                    启用
                  </label>
                  <label className="settings-login-toggle">
                    <input
                      type="checkbox"
                      checked={Boolean(t.require_login)}
                      disabled={isToggling}
                      onChange={(e) => {
                        void onRequireLoginChange(t.name, e.target.checked)
                      }}
                    />
                    需要登录
                  </label>
                  {canDelete && (
                    <button
                      type="button"
                      className="btn danger sm"
                      disabled={isDeleting || isToggling}
                      onClick={() => {
                        void onDelete(t)
                      }}
                    >
                      {isDeleting ? '删除中…' : '删除'}
                    </button>
                  )}
                </span>
              </li>
            )
          })}
        </ul>
      )}
      {showAddForm && (
        <form className="settings-form" onSubmit={onAddSubmit}>
          <h2 className="settings-subheading">添加工具</h2>
          {openConnectorIds.length > 1 && (
            <label className="settings-field">
              <span className="settings-field-label">Connector</span>
              <select
                className="settings-select"
                value={formConnectorId}
                onChange={(e) => setFormConnectorId(e.target.value)}
                disabled={submitting}
              >
                {openConnectorIds.map((id) => (
                  <option key={id} value={id}>
                    {id}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label className="settings-field">
            <span className="settings-field-label">名称</span>
            <input
              className="settings-input"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              disabled={submitting}
              required
            />
          </label>
          <label className="settings-field">
            <span className="settings-field-label">HTTP 方法</span>
            <select
              className="settings-select"
              value={form.method}
              onChange={(e) => setForm((f) => ({ ...f, method: e.target.value }))}
              disabled={submitting}
            >
              {HTTP_METHODS.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </label>
          <label className="settings-field">
            <span className="settings-field-label">路径</span>
            <input
              className="settings-input"
              value={form.path}
              onChange={(e) => setForm((f) => ({ ...f, path: e.target.value }))}
              disabled={submitting}
              required
              placeholder="/items/{id}"
            />
          </label>
          <label className="settings-field">
            <span className="settings-field-label">描述</span>
            <input
              className="settings-input"
              value={form.description}
              onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
              disabled={submitting}
            />
          </label>
          <label className="settings-field">
            <span className="settings-field-label">input_schema</span>
            <textarea
              className="settings-textarea"
              value={form.schema}
              onChange={(e) => setForm((f) => ({ ...f, schema: e.target.value }))}
              disabled={submitting}
              rows={4}
            />
          </label>
          <button type="submit" className="btn primary" disabled={submitting}>
            {submitting ? '提交中…' : '添加'}
          </button>
        </form>
      )}
    </div>
  )
}
