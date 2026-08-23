import { type ChangeEvent, type FormEvent, useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  getConnector,
  listTools,
  putConnector,
  type ApiError,
  type ConnectorAuth,
  type ConnectorInfo,
  type ImportFormat,
  type ToolInfo,
} from '../api'
import {
  detectedFormatHint,
  detectImportFormat,
  importFormatLabel,
  type DetectedImportFormat,
} from '../specImport'
import {
  buildConnectorAuth,
  connectorToForm as pluginConnectorToForm,
  parseLineList,
  type PluginAuthMode,
} from './PluginSettings'

export interface OpenApiFormState {
  id: string
  baseUrl: string
  importFormat: ImportFormat
  authMode: PluginAuthMode
  authHeadersText: string
  authPassthroughText: string
  requireApprovalText: string
  requireLoginText: string
  executionCallbackUrl: string
}

const EMPTY_FORM: OpenApiFormState = {
  id: '',
  baseUrl: '',
  importFormat: 'auto',
  authMode: 'static',
  authHeadersText: '',
  authPassthroughText: '',
  requireApprovalText: '',
  requireLoginText: '',
  executionCallbackUrl: '',
}

export function openApiConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source !== 'spec' && t.source !== 'extra') continue
    if (!t.connector_id) continue
    if (seen.has(t.connector_id)) continue
    seen.add(t.connector_id)
    ordered.push(t.connector_id)
  }
  return ordered
}

export function connectorToForm(c: ConnectorInfo): OpenApiFormState {
  const plugin = pluginConnectorToForm(c)
  return {
    id: plugin.id,
    baseUrl: plugin.baseUrl,
    importFormat: 'auto',
    authMode: plugin.authMode,
    authHeadersText: plugin.authHeadersText,
    authPassthroughText: plugin.authPassthroughText,
    requireApprovalText: plugin.requireApprovalText,
    requireLoginText: plugin.requireLoginText,
    executionCallbackUrl: c.execution_callback_url ?? '',
  }
}

export function validateOpenApiForm(
  form: OpenApiFormState,
  options: { editing: boolean; hasNewSpec: boolean },
):
  | {
      ok: true
      baseUrl: string
      auth: ConnectorAuth
      requireApproval: string[]
      requireLogin: string[]
      executionCallbackUrl: string
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
  if (!options.editing && !options.hasNewSpec) {
    return { ok: false, code: 'invalid_request', message: '请上传接口文档' }
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
    executionCallbackUrl: form.executionCallbackUrl.trim(),
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

interface OpenApiConnectorRow {
  info: ConnectorInfo
  toolCount: number
}

export function OpenApiSettings() {
  const [rows, setRows] = useState<OpenApiConnectorRow[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [editing, setEditing] = useState(false)
  const [form, setForm] = useState<OpenApiFormState>(EMPTY_FORM)
  const [formError, setFormError] = useState<string | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [specContent, setSpecContent] = useState<string | null>(null)
  const [specFileName, setSpecFileName] = useState<string | null>(null)
  const [detectedFormat, setDetectedFormat] = useState<DetectedImportFormat | null>(null)

  const load = useCallback(async () => {
    try {
      const tools = await listTools()
      const ids = openApiConnectorIds(tools)
      const toolCountByConnector = new Map<string, number>()
      for (const t of tools) {
        if (t.source !== 'spec' && t.source !== 'extra') continue
        if (!t.connector_id) continue
        toolCountByConnector.set(t.connector_id, (toolCountByConnector.get(t.connector_id) ?? 0) + 1)
      }
      const connectors = await Promise.all(ids.map((id) => getConnector(id)))
      const next: OpenApiConnectorRow[] = connectors.map((info) => ({
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

  const resetSpecState = () => {
    setSpecContent(null)
    setSpecFileName(null)
    setDetectedFormat(null)
  }

  const openCreate = () => {
    setEditing(false)
    setForm(EMPTY_FORM)
    resetSpecState()
    setFormError(null)
    setStatus(null)
    setFormOpen(true)
  }

  const openEdit = (c: ConnectorInfo) => {
    setEditing(true)
    setForm(connectorToForm(c))
    resetSpecState()
    setFormError(null)
    setStatus(null)
    setFormOpen(true)
  }

  const closeForm = () => {
    if (submitting) return
    setFormOpen(false)
  }

  const onSpecFileChange = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      const text = typeof reader.result === 'string' ? reader.result : ''
      setSpecContent(text)
      setSpecFileName(file.name)
      setDetectedFormat(detectImportFormat(text))
    }
    reader.onerror = () => {
      setFormError('读取文件失败')
      resetSpecState()
    }
    reader.readAsText(file)
  }

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    const hasNewSpec = specContent != null
    const validated = validateOpenApiForm(form, { editing, hasNewSpec })
    if (!validated.ok) {
      setFormError(`${validated.code}: ${validated.message}`)
      return
    }
    setSubmitting(true)
    setFormError(null)
    try {
      await putConnector(form.id.trim(), {
        type: 'openapi',
        base_url: validated.baseUrl,
        auth: validated.auth,
        import_format: form.importFormat,
        spec_content: hasNewSpec ? specContent! : undefined,
        execution_callback_url:
          validated.executionCallbackUrl !== '' ? validated.executionCallbackUrl : undefined,
        require_approval:
          validated.requireApproval.length > 0 ? validated.requireApproval : undefined,
        require_login: validated.requireLogin.length > 0 ? validated.requireLogin : undefined,
      })
      setFormOpen(false)
      setStatus(`已保存 ${form.id.trim()}`)
      setError(null)
      resetSpecState()
      await load()
    } catch (err) {
      setFormError(apiErrorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const loadFailed = rows === null && error !== null
  const busy = submitting
  const specHint =
    specContent != null
      ? detectedFormatHint(detectedFormat)
      : editing
        ? '未选择新文件时将沿用已有文档'
        : null

  return (
    <div className="settings-section settings-openapi">
      <h1 className="settings-heading">OpenAPI</h1>
      <p className="settings-meta">
        上传企业接口文档（OpenAPI 3、Swagger 2、Postman Collection v2.1），服务端自动转换为 OpenAPI 3
        并发现全部 Tools。若文档内 host 不正确，以填写的 base_url 为准。工具在{' '}
        <Link to="/settings/tools" className="settings-link">
          Tools
        </Link>{' '}
        中管理。
      </p>
      {loadFailed && <p className="settings-error">无法加载 OpenAPI：{error}</p>}
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
          {rows.length === 0 && <p className="settings-empty">尚未注册 OpenAPI Connector</p>}
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
                const formatLabel = importFormatLabel(info.import_format_detected)
                return (
                  <li key={info.id} className="settings-list-item settings-openapi-row">
                    <span className="settings-tool-line">
                      <span className="settings-tool-title">{info.id}</span>
                      <span className="settings-tool-sub">
                        {info.base_url ?? '—'} · auth {authModeLabel(info.auth?.mode)}
                        {info.import_format_detected
                          ? ` · ${formatLabel}`
                          : ''}
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
            onClick={(ev) => ev.stopPropagation()}
            role="dialog"
            aria-label={editing ? '编辑 OpenAPI Connector' : '添加 OpenAPI Connector'}
          >
            <div className="settings-drawer-head">
              <h2 className="settings-subheading">
                {editing ? '编辑 OpenAPI Connector' : '添加 OpenAPI Connector'}
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
                  placeholder="ticket-api"
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
                  placeholder="https://api.example.com"
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">接口文档（.json / .yaml / .yml）</span>
                <input
                  className="settings-input"
                  type="file"
                  accept=".json,.yaml,.yml"
                  disabled={submitting}
                  onChange={onSpecFileChange}
                />
                {specFileName && <span className="settings-muted">已选：{specFileName}</span>}
                {specHint && <span className="settings-muted">{specHint}</span>}
              </label>
              <label className="settings-field">
                <span className="settings-field-label">import_format</span>
                <select
                  className="settings-select"
                  value={form.importFormat}
                  onChange={(e) => {
                    const v = e.target.value
                    const importFormat: ImportFormat =
                      v === 'openapi3' || v === 'swagger2' || v === 'postman' ? v : 'auto'
                    setForm((f) => ({ ...f, importFormat }))
                  }}
                  disabled={submitting}
                >
                  <option value="auto">自动识别</option>
                  <option value="openapi3">OpenAPI 3</option>
                  <option value="swagger2">Swagger 2</option>
                  <option value="postman">Postman Collection</option>
                </select>
              </label>
              <label className="settings-field">
                <span className="settings-field-label">execution_callback_url（可选）</span>
                <input
                  className="settings-input"
                  value={form.executionCallbackUrl}
                  onChange={(e) => setForm((f) => ({ ...f, executionCallbackUrl: e.target.value }))}
                  disabled={submitting}
                  placeholder="https://hooks.example.com/baize"
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
