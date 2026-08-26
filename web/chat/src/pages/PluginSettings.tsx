import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  getConnector,
  listTools,
  putConnector,
  type ApiError,
  type ConnectorAuth,
  type ConnectorInfo,
  type ToolInfo,
} from '../api'
import { confirmAndDeleteConnector, defaultConnectorDeleteDeps } from './connectorDelete'

export type PluginAuthMode = 'static' | 'passthrough' | 'vault_ref'

export interface PluginFormState {
  id: string
  baseUrl: string
  authMode: PluginAuthMode
  authHeadersText: string
  authPassthroughText: string
  requireApprovalText: string
  requireLoginText: string
}

const EMPTY_FORM: PluginFormState = {
  id: '',
  baseUrl: '',
  authMode: 'static',
  authHeadersText: '',
  authPassthroughText: '',
  requireApprovalText: '',
  requireLoginText: '',
}

export function pluginConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source !== 'plugin') continue
    if (!t.connector_id) continue
    if (seen.has(t.connector_id)) continue
    seen.add(t.connector_id)
    ordered.push(t.connector_id)
  }
  return ordered
}

export function formatKeyValueMap(map: Record<string, string> | undefined): string {
  if (!map) return ''
  return Object.entries(map)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}

export function formatStringList(list: string[] | undefined): string {
  if (!list || list.length === 0) return ''
  return list.join('\n')
}

export function parseKeyValueLines(
  text: string,
): { ok: true; value: Record<string, string> } | { ok: false; message: string } {
  const trimmed = text.trim()
  if (trimmed === '') return { ok: true, value: {} }
  const out: Record<string, string> = {}
  for (const line of text.split('\n')) {
    const row = line.trim()
    if (row === '') continue
    const eq = row.indexOf('=')
    if (eq <= 0) {
      return { ok: false, message: `无效键值行：${row}` }
    }
    const key = row.slice(0, eq).trim()
    const value = row.slice(eq + 1).trim()
    if (!key) return { ok: false, message: `无效键值行：${row}` }
    out[key] = value
  }
  return { ok: true, value: out }
}

export function parseLineList(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s !== '')
}

export function buildConnectorAuth(
  mode: PluginAuthMode,
  headersText: string,
  passthroughText: string,
):
  | { ok: true; auth: ConnectorAuth }
  | { ok: false; code: string; message: string } {
  const auth: ConnectorAuth = { mode }
  if (mode === 'static') {
    const parsed = parseKeyValueLines(headersText)
    if (!parsed.ok) {
      return { ok: false, code: 'invalid_request', message: parsed.message }
    }
    if (Object.keys(parsed.value).length > 0) {
      auth.static = { headers: parsed.value }
    }
  } else if (mode === 'passthrough') {
    const headers = parseLineList(passthroughText)
    if (headers.length > 0) {
      auth.passthrough = { headers }
    }
  } else {
    const parsed = parseKeyValueLines(headersText)
    if (!parsed.ok) {
      return { ok: false, code: 'invalid_request', message: parsed.message }
    }
    if (Object.keys(parsed.value).length > 0) {
      auth.vault_ref = { headers: parsed.value }
    }
  }
  return { ok: true, auth }
}

export function validatePluginForm(
  form: PluginFormState,
):
  | {
      ok: true
      baseUrl: string
      auth: ConnectorAuth
      requireApproval: string[]
      requireLogin: string[]
    }
  | { ok: false; code: string; message: string } {
  const id = form.id.trim()
  if (!id) {
    return { ok: false, code: 'invalid_request', message: 'Connector ID 不能为空' }
  }
  const baseUrl = form.baseUrl.trim()
  if (!baseUrl) {
    return { ok: false, code: 'invalid_request', message: 'base_url 不能为空' }
  }
  const authBuilt = buildConnectorAuth(form.authMode, form.authHeadersText, form.authPassthroughText)
  if (!authBuilt.ok) {
    return authBuilt
  }
  return {
    ok: true,
    baseUrl,
    auth: authBuilt.auth,
    requireApproval: parseLineList(form.requireApprovalText),
    requireLogin: parseLineList(form.requireLoginText),
  }
}

export function connectorToForm(c: ConnectorInfo): PluginFormState {
  const mode = (c.auth?.mode ?? 'static') as PluginAuthMode
  const authMode: PluginAuthMode =
    mode === 'passthrough' || mode === 'vault_ref' ? mode : 'static'
  return {
    id: c.id,
    baseUrl: c.base_url ?? '',
    authMode,
    authHeadersText:
      authMode === 'vault_ref'
        ? formatKeyValueMap(c.auth?.vault_ref?.headers)
        : formatKeyValueMap(c.auth?.static?.headers),
    authPassthroughText: formatStringList(c.auth?.passthrough?.headers),
    requireApprovalText: formatStringList(c.require_approval),
    requireLoginText: formatStringList(c.require_login),
  }
}

function apiErrorMessage(err: unknown): string {
  if (err && typeof err === 'object' && 'code' in err && 'message' in err) {
    const e = err as ApiError
    return `${e.code}: ${e.message}`
  }
  return err instanceof Error ? err.message : String(err)
}

function authModeLabel(mode: string | undefined): string {
  switch (mode) {
    case 'passthrough':
      return 'passthrough'
    case 'vault_ref':
      return 'vault_ref'
    case 'capture':
      return 'capture'
    case 'static':
    case '':
    case undefined:
      return 'static'
    default:
      return mode
  }
}

interface PluginConnectorRow {
  info: ConnectorInfo
  toolCount: number
}

export function PluginSettings() {
  const [rows, setRows] = useState<PluginConnectorRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<PluginFormState>(EMPTY_FORM)
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const tools = await listTools()
      const ids = pluginConnectorIds(tools)
      const toolCountByConnector = new Map<string, number>()
      for (const t of tools) {
        if (t.source !== 'plugin' || !t.connector_id) continue
        toolCountByConnector.set(t.connector_id, (toolCountByConnector.get(t.connector_id) ?? 0) + 1)
      }
      const connectors = await Promise.all(ids.map((id) => getConnector(id)))
      const next: PluginConnectorRow[] = connectors.map((info) => ({
        info,
        toolCount: toolCountByConnector.get(info.id) ?? info.tools?.length ?? 0,
      }))
      setRows(next)
      setError(null)
    } catch (err) {
      setRows(null)
      setError(apiErrorMessage(err))
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const openCreate = () => {
    setEditing(false)
    setForm(EMPTY_FORM)
    setFormError(null)
    setStatus(null)
    setFormOpen(true)
  }

  const openEdit = (c: ConnectorInfo) => {
    setEditing(true)
    setForm(connectorToForm(c))
    setFormError(null)
    setStatus(null)
    setFormOpen(true)
  }

  const closeForm = () => {
    if (submitting) return
    setFormOpen(false)
  }

  const onDelete = async (id: string) => {
    setDeleting(id)
    setStatus(null)
    try {
      const deleted = await confirmAndDeleteConnector(id, defaultConnectorDeleteDeps)
      if (!deleted) return
      setStatus(`已删除 ${id}`)
      setError(null)
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setDeleting(null)
    }
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const validated = validatePluginForm(form)
    if (!validated.ok) {
      setFormError(`${validated.code}: ${validated.message}`)
      return
    }
    setSubmitting(true)
    setFormError(null)
    try {
      await putConnector(form.id.trim(), {
        type: 'http',
        base_url: validated.baseUrl,
        auth: validated.auth,
        require_approval:
          validated.requireApproval.length > 0 ? validated.requireApproval : undefined,
        require_login: validated.requireLogin.length > 0 ? validated.requireLogin : undefined,
      })
      setFormOpen(false)
      setStatus(`已保存 ${form.id.trim()}`)
      setError(null)
      await load()
    } catch (err) {
      setFormError(apiErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const loadFailed = rows === null && error !== null
  const busy = submitting || deleting !== null

  return (
    <div className="settings-section settings-plugins">
      <h1 className="settings-heading">插件</h1>
      <p className="settings-meta">
        注册 HTTP 插件侧车（协议 v0：healthz + <code>/v0/tools</code> + invoke）。工具在{' '}
        <Link to="/settings/tools" className="settings-link">
          Tools
        </Link>{' '}
        中管理。参考实现见仓库 <code>examples/http-plugin</code>。
      </p>
      {loadFailed && <p className="settings-error">无法加载插件：{error}</p>}
      {!loadFailed && error && <p className="settings-error">{error}</p>}
      {!loadFailed && status && <p className="settings-muted">{status}</p>}
      {rows === null && !loadFailed && <p className="settings-muted">加载中…</p>}
      {rows !== null && (
        <>
          <div className="settings-toolbar">
            <button type="button" className="btn primary sm" disabled={busy} onClick={openCreate}>
              添加 Connector
            </button>
          </div>
          {rows.length === 0 && <p className="settings-empty">尚未注册 HTTP 插件 Connector</p>}
          {rows.length > 0 && (
            <ul className="settings-list">
              {rows.map(({ info, toolCount }) => {
                const requireParts: string[] = []
                if (info.require_approval && info.require_approval.length > 0) {
                  requireParts.push(`审批：${info.require_approval.join(', ')}`)
                }
                if (info.require_login && info.require_login.length > 0) {
                  requireParts.push(`登录：${info.require_login.join(', ')}`)
                }
                return (
                  <li key={info.id} className="settings-list-item settings-plugins-row">
                    <span className="settings-tool-line">
                      <span className="settings-tool-title">{info.id}</span>
                      <span className="settings-tool-sub">
                        {info.base_url ?? '—'} · auth {authModeLabel(info.auth?.mode)}
                      </span>
                      {requireParts.length > 0 && (
                        <span className="settings-tool-desc">{requireParts.join(' · ')}</span>
                      )}
                    </span>
                    <span className="settings-tool-actions">
                      <span className="settings-badge">{toolCount} 个工具</span>
                      <Link to="/settings/tools" className="btn ghost sm">
                        Tools
                      </Link>
                      <button
                        type="button"
                        className="btn ghost sm"
                        disabled={busy}
                        onClick={() => openEdit(info)}
                      >
                        编辑
                      </button>
                      <button
                        type="button"
                        className="btn danger sm"
                        disabled={busy}
                        onClick={() => {
                          void onDelete(info.id)
                        }}
                      >
                        {deleting === info.id ? '删除中…' : '删除'}
                      </button>
                    </span>
                  </li>
                )
              })}
            </ul>
          )}
        </>
      )}
      {formOpen && (
        <div className="settings-drawer-backdrop" onClick={closeForm}>
          <aside
            className="settings-drawer"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-label={editing ? '编辑 HTTP 插件 Connector' : '添加 HTTP 插件 Connector'}
          >
            <div className="settings-drawer-head">
              <h2 className="settings-subheading">
                {editing ? '编辑 HTTP 插件 Connector' : '添加 HTTP 插件 Connector'}
              </h2>
              <button type="button" className="btn ghost sm" onClick={closeForm} disabled={submitting}>
                关闭
              </button>
            </div>
            {formError && <p className="settings-error">{formError}</p>}
            <form className="settings-form" onSubmit={onSubmit}>
              <label className="settings-field">
                <span className="settings-field-label">Connector ID</span>
                <input
                  className="settings-input"
                  value={form.id}
                  onChange={(e) => setForm((f) => ({ ...f, id: e.target.value }))}
                  disabled={submitting || editing}
                  required
                  placeholder="legacy-sidecar"
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">base_url</span>
                <input
                  className="settings-input"
                  value={form.baseUrl}
                  onChange={(e) => setForm((f) => ({ ...f, baseUrl: e.target.value }))}
                  disabled={submitting}
                  required
                  placeholder="http://127.0.0.1:19090"
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">auth.mode</span>
                <select
                  className="settings-select"
                  value={form.authMode}
                  onChange={(e) => {
                    const mode = e.target.value
                    const authMode: PluginAuthMode =
                      mode === 'passthrough' || mode === 'vault_ref' ? mode : 'static'
                    setForm((f) => ({ ...f, authMode }))
                  }}
                  disabled={submitting}
                >
                  <option value="static">static（固定 headers）</option>
                  <option value="passthrough">passthrough（透传请求头）</option>
                  <option value="vault_ref">vault_ref（密钥引用）</option>
                </select>
              </label>
              {form.authMode === 'passthrough' ? (
                <label className="settings-field">
                  <span className="settings-field-label">passthrough headers（每行一个 header 名）</span>
                  <textarea
                    className="settings-textarea"
                    value={form.authPassthroughText}
                    onChange={(e) => setForm((f) => ({ ...f, authPassthroughText: e.target.value }))}
                    disabled={submitting}
                    rows={3}
                    placeholder="Authorization"
                  />
                </label>
              ) : (
                <label className="settings-field">
                  <span className="settings-field-label">
                    {form.authMode === 'vault_ref' ? 'vault_ref headers' : 'static headers'}（每行
                    KEY=VALUE）
                  </span>
                  <textarea
                    className="settings-textarea"
                    value={form.authHeadersText}
                    onChange={(e) => setForm((f) => ({ ...f, authHeadersText: e.target.value }))}
                    disabled={submitting}
                    rows={3}
                    placeholder={
                      form.authMode === 'vault_ref'
                        ? 'Authorization=vault:prod/api-key'
                        : 'Authorization=Bearer ${API_KEY}'
                    }
                  />
                </label>
              )}
              <label className="settings-field">
                <span className="settings-field-label">require_approval（每行一个工具名）</span>
                <textarea
                  className="settings-textarea"
                  value={form.requireApprovalText}
                  onChange={(e) => setForm((f) => ({ ...f, requireApprovalText: e.target.value }))}
                  disabled={submitting}
                  rows={3}
                  placeholder="create_ticket"
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">require_login（每行一个工具名，可选）</span>
                <textarea
                  className="settings-textarea"
                  value={form.requireLoginText}
                  onChange={(e) => setForm((f) => ({ ...f, requireLoginText: e.target.value }))}
                  disabled={submitting}
                  rows={3}
                  placeholder="sensitive_read"
                />
              </label>
              <button type="submit" className="btn primary" disabled={submitting}>
                {submitting ? '保存中…' : '保存'}
              </button>
            </form>
          </aside>
        </div>
      )}
    </div>
  )
}
