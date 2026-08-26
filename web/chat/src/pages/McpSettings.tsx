import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  getConnector,
  listTools,
  putConnector,
  type ApiError,
  type ConnectorInfo,
  type MCPConfig,
  type ToolInfo,
} from '../api'
import { confirmAndDeleteConnector, defaultConnectorDeleteDeps } from './connectorDelete'

export interface McpFormState {
  id: string
  transport: 'stdio' | 'http'
  command: string
  argsText: string
  envText: string
  url: string
  headersText: string
  requireApprovalText: string
}

const EMPTY_FORM: McpFormState = {
  id: '',
  transport: 'stdio',
  command: '',
  argsText: '',
  envText: '',
  url: '',
  headersText: '',
  requireApprovalText: '',
}

export function mcpConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source !== 'mcp') continue
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

export function parseKeyValueLines(text: string): { ok: true; value: Record<string, string> } | { ok: false; message: string } {
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

export function parseArgsText(text: string): string[] {
  const trimmed = text.trim()
  if (trimmed === '') return []
  if (trimmed.includes('\n')) {
    return trimmed
      .split('\n')
      .map((s) => s.trim())
      .filter((s) => s !== '')
  }
  return trimmed.split(/\s+/).filter((s) => s !== '')
}

export function parseLineList(text: string): string[] {
  return text
    .split('\n')
    .map((s) => s.trim())
    .filter((s) => s !== '')
}

export function validateMcpForm(
  form: McpFormState,
): { ok: true; mcp: MCPConfig; requireApproval: string[] } | { ok: false; code: string; message: string } {
  const id = form.id.trim()
  if (!id) {
    return { ok: false, code: 'invalid_request', message: 'Connector ID 不能为空' }
  }
  const requireApproval = parseLineList(form.requireApprovalText)
  if (form.transport === 'stdio') {
    const command = form.command.trim()
    if (!command) {
      return { ok: false, code: 'invalid_request', message: 'stdio 需要 command' }
    }
    const envParsed = parseKeyValueLines(form.envText)
    if (!envParsed.ok) {
      return { ok: false, code: 'invalid_request', message: envParsed.message }
    }
    return {
      ok: true,
      mcp: {
        transport: 'stdio',
        command,
        args: parseArgsText(form.argsText),
        env: Object.keys(envParsed.value).length > 0 ? envParsed.value : undefined,
      },
      requireApproval,
    }
  }
  const url = form.url.trim()
  if (!url) {
    return { ok: false, code: 'invalid_request', message: 'http 需要 url' }
  }
  const headersParsed = parseKeyValueLines(form.headersText)
  if (!headersParsed.ok) {
    return { ok: false, code: 'invalid_request', message: headersParsed.message }
  }
  return {
    ok: true,
    mcp: {
      transport: 'http',
      url,
      headers: Object.keys(headersParsed.value).length > 0 ? headersParsed.value : undefined,
    },
    requireApproval,
  }
}

export function connectorToForm(c: ConnectorInfo): McpFormState {
  const mcp = c.mcp
  return {
    id: c.id,
    transport: mcp?.transport === 'http' ? 'http' : 'stdio',
    command: mcp?.command ?? '',
    argsText: formatStringList(mcp?.args),
    envText: formatKeyValueMap(mcp?.env),
    url: mcp?.url ?? '',
    headersText: formatKeyValueMap(mcp?.headers),
    requireApprovalText: formatStringList(c.require_approval),
  }
}

function apiErrorMessage(err: unknown): string {
  if (err && typeof err === 'object' && 'code' in err && 'message' in err) {
    const e = err as ApiError
    return `${e.code}: ${e.message}`
  }
  return err instanceof Error ? err.message : String(err)
}

interface McpConnectorRow {
  info: ConnectorInfo
  toolCount: number
}

export function McpSettings() {
  const [rows, setRows] = useState<McpConnectorRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<McpFormState>(EMPTY_FORM)
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [deleting, setDeleting] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const tools = await listTools()
      const ids = mcpConnectorIds(tools)
      const toolCountByConnector = new Map<string, number>()
      for (const t of tools) {
        if (t.source !== 'mcp' || !t.connector_id) continue
        toolCountByConnector.set(t.connector_id, (toolCountByConnector.get(t.connector_id) ?? 0) + 1)
      }
      const connectors = await Promise.all(ids.map((id) => getConnector(id)))
      const next: McpConnectorRow[] = connectors.map((info) => ({
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
    const validated = validateMcpForm(form)
    if (!validated.ok) {
      setFormError(`${validated.code}: ${validated.message}`)
      return
    }
    setSubmitting(true)
    setFormError(null)
    try {
      await putConnector(form.id.trim(), {
        type: 'mcp',
        mcp: validated.mcp,
        require_approval: validated.requireApproval.length > 0 ? validated.requireApproval : undefined,
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
    <div className="settings-section settings-mcp">
      <h1 className="settings-heading">MCP</h1>
      <p className="settings-meta">
        注册 Model Context Protocol Server；工具在{' '}
        <Link to="/settings/tools" className="settings-link">
          Tools
        </Link>{' '}
        中管理。
      </p>
      {loadFailed && <p className="settings-error">无法加载 MCP：{error}</p>}
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
          {rows.length === 0 && <p className="settings-empty">尚未注册 MCP Connector</p>}
          {rows.length > 0 && (
            <ul className="settings-list">
              {rows.map(({ info, toolCount }) => {
                const transport = info.mcp?.transport ?? 'stdio'
                const summary =
                  transport === 'http'
                    ? info.mcp?.url ?? '—'
                    : [info.mcp?.command, ...(info.mcp?.args ?? [])].filter(Boolean).join(' ') || '—'
                return (
                  <li key={info.id} className="settings-list-item settings-mcp-row">
                    <span className="settings-tool-line">
                      <span className="settings-tool-title">{info.id}</span>
                      <span className="settings-tool-sub">
                        {transport} · {summary}
                      </span>
                      {info.require_approval && info.require_approval.length > 0 && (
                        <span className="settings-tool-desc">
                          需审批：{info.require_approval.join(', ')}
                        </span>
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
            aria-label={editing ? '编辑 MCP Connector' : '添加 MCP Connector'}
          >
            <div className="settings-drawer-head">
              <h2 className="settings-subheading">{editing ? '编辑 MCP Connector' : '添加 MCP Connector'}</h2>
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
                  placeholder="analytics"
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">传输</span>
                <select
                  className="settings-select"
                  value={form.transport}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      transport: e.target.value === 'http' ? 'http' : 'stdio',
                    }))
                  }
                  disabled={submitting}
                >
                  <option value="stdio">stdio（子进程）</option>
                  <option value="http">http（Streamable HTTP）</option>
                </select>
              </label>
              {form.transport === 'stdio' ? (
                <>
                  <label className="settings-field">
                    <span className="settings-field-label">command</span>
                    <input
                      className="settings-input"
                      value={form.command}
                      onChange={(e) => setForm((f) => ({ ...f, command: e.target.value }))}
                      disabled={submitting}
                      required
                      placeholder="npx"
                    />
                  </label>
                  <label className="settings-field">
                    <span className="settings-field-label">args（空格或每行一个）</span>
                    <textarea
                      className="settings-textarea"
                      value={form.argsText}
                      onChange={(e) => setForm((f) => ({ ...f, argsText: e.target.value }))}
                      disabled={submitting}
                      rows={3}
                      placeholder="@bytebase/dbhub"
                    />
                  </label>
                  <label className="settings-field">
                    <span className="settings-field-label">env（每行 KEY=VALUE）</span>
                    <textarea
                      className="settings-textarea"
                      value={form.envText}
                      onChange={(e) => setForm((f) => ({ ...f, envText: e.target.value }))}
                      disabled={submitting}
                      rows={3}
                      placeholder="DSN=postgres://..."
                    />
                  </label>
                </>
              ) : (
                <>
                  <label className="settings-field">
                    <span className="settings-field-label">url</span>
                    <input
                      className="settings-input"
                      value={form.url}
                      onChange={(e) => setForm((f) => ({ ...f, url: e.target.value }))}
                      disabled={submitting}
                      required
                      placeholder="https://mcp.example.com"
                    />
                  </label>
                  <label className="settings-field">
                    <span className="settings-field-label">headers（每行 KEY=VALUE）</span>
                    <textarea
                      className="settings-textarea"
                      value={form.headersText}
                      onChange={(e) => setForm((f) => ({ ...f, headersText: e.target.value }))}
                      disabled={submitting}
                      rows={3}
                      placeholder="Authorization=Bearer ${API_KEY}"
                    />
                  </label>
                </>
              )}
              <label className="settings-field">
                <span className="settings-field-label">require_approval（每行一个工具名）</span>
                <textarea
                  className="settings-textarea"
                  value={form.requireApprovalText}
                  onChange={(e) => setForm((f) => ({ ...f, requireApprovalText: e.target.value }))}
                  disabled={submitting}
                  rows={3}
                  placeholder="dangerous_tool"
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
