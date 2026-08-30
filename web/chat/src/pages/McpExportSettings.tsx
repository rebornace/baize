import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  createMCPExportIdentity,
  createMCPExportKey,
  deleteMCPExportIdentity,
  getMCPExportSettings,
  listMCPExportIdentities,
  listMCPExportKeys,
  patchMCPExportIdentity,
  revokeMCPExportKey,
  type MCPExportIdentity,
  type MCPExportKey,
  type MCPExportSettings as MCPExportSettingsInfo,
} from '../api'
import { formatKeyValueMap, parseKeyValueLines } from './McpSettings'

export interface IdentityFormState {
  name: string
  scheme: string
  headersText: string
}

const EMPTY_IDENTITY_FORM: IdentityFormState = {
  name: '',
  scheme: '',
  headersText: '',
}

export function mcpExportEndpointUrl(origin: string, endpointPath: string): string {
  const base = origin.replace(/\/$/, '')
  const path = endpointPath.startsWith('/') ? endpointPath : `/${endpointPath}`
  return `${base}${path}`
}

export function identityToForm(identity: MCPExportIdentity): IdentityFormState {
  return {
    name: identity.name,
    scheme: identity.scheme ?? '',
    headersText: formatKeyValueMap(identity.headers),
  }
}

export function validateIdentityForm(
  form: IdentityFormState,
):
  | { ok: true; name: string; scheme: string; headers: Record<string, string> }
  | { ok: false; message: string } {
  const name = form.name.trim()
  if (!name) {
    return { ok: false, message: '名称不能为空' }
  }
  const headersParsed = parseKeyValueLines(form.headersText)
  if (!headersParsed.ok) {
    return { ok: false, message: headersParsed.message }
  }
  return {
    ok: true,
    name,
    scheme: form.scheme.trim(),
    headers: headersParsed.value,
  }
}

function apiErrorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function isKeyActive(key: MCPExportKey): boolean {
  return key.revoked_at == null || key.revoked_at === ''
}

export function McpExportSettings() {
  const [settings, setSettings] = useState<MCPExportSettingsInfo | null>(null)
  const [identities, setIdentities] = useState<MCPExportIdentity[]>([])
  const [keys, setKeys] = useState<MCPExportKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [status, setStatus] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const [createForm, setCreateForm] = useState<IdentityFormState>(EMPTY_IDENTITY_FORM)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editForm, setEditForm] = useState<IdentityFormState>(EMPTY_IDENTITY_FORM)

  const [keyName, setKeyName] = useState('')
  const [keyIdentityId, setKeyIdentityId] = useState('')
  const [tokenModal, setTokenModal] = useState<{ name: string; token: string } | null>(null)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const [settingsBody, ids, keyList] = await Promise.all([
        getMCPExportSettings(),
        listMCPExportIdentities(),
        listMCPExportKeys(),
      ])
      setSettings(settingsBody)
      setIdentities(ids)
      setKeys(keyList)
      setError(null)
      setKeyIdentityId((prev) => {
        if (prev && ids.some((i) => i.id === prev)) return prev
        return ids[0]?.id ?? ''
      })
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  const endpointUrl =
    settings != null
      ? mcpExportEndpointUrl(
          typeof window !== 'undefined' ? window.location.origin : '',
          settings.endpoint_path,
        )
      : ''

  const onCopyEndpoint = async () => {
    if (!endpointUrl) return
    try {
      await navigator.clipboard.writeText(endpointUrl)
      setStatus(`已复制 endpoint：${endpointUrl}`)
      setError(null)
    } catch (err) {
      setError(`复制失败：${apiErrorMessage(err)}`)
    }
  }

  const onCreateIdentity = async (e: FormEvent) => {
    e.preventDefault()
    const validated = validateIdentityForm(createForm)
    if (!validated.ok) {
      setError(validated.message)
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      await createMCPExportIdentity({
        name: validated.name,
        scheme: validated.scheme || undefined,
        headers: Object.keys(validated.headers).length > 0 ? validated.headers : undefined,
      })
      setCreateForm(EMPTY_IDENTITY_FORM)
      setStatus('已创建导出身份')
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const startEdit = (identity: MCPExportIdentity) => {
    setEditingId(identity.id)
    setEditForm(identityToForm(identity))
    setError(null)
    setStatus(null)
  }

  const onSaveEdit = async (e: FormEvent) => {
    e.preventDefault()
    if (!editingId) return
    const validated = validateIdentityForm(editForm)
    if (!validated.ok) {
      setError(validated.message)
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      await patchMCPExportIdentity(editingId, {
        name: validated.name,
        scheme: validated.scheme,
        headers: validated.headers,
      })
      setEditingId(null)
      setEditForm(EMPTY_IDENTITY_FORM)
      setStatus('已更新导出身份')
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onDeleteIdentity = async (id: string, name: string) => {
    if (!window.confirm(`删除导出身份「${name}」？其下的 Key 也会被删除。`)) return
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      await deleteMCPExportIdentity(id)
      if (editingId === id) {
        setEditingId(null)
        setEditForm(EMPTY_IDENTITY_FORM)
      }
      setStatus(`已删除身份 ${name}`)
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onCreateKey = async (e: FormEvent) => {
    e.preventDefault()
    const name = keyName.trim()
    const identityId = keyIdentityId.trim()
    if (!name) {
      setError('Key 名称不能为空')
      return
    }
    if (!identityId) {
      setError('请选择绑定的导出身份')
      return
    }
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      const created = await createMCPExportKey({ name, identity_id: identityId })
      setKeyName('')
      setTokenModal({ name: created.name, token: created.token })
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const onRevokeKey = async (key: MCPExportKey) => {
    if (!window.confirm(`撤销 Key「${key.name}」（${key.prefix}…）？此操作不可恢复。`)) return
    setBusy(true)
    setError(null)
    setStatus(null)
    try {
      await revokeMCPExportKey(key.id)
      setStatus(`已撤销 Key ${key.name}`)
      await load()
    } catch (err) {
      setError(apiErrorMessage(err))
    } finally {
      setBusy(false)
    }
  }

  const identityName = (id: string) => identities.find((i) => i.id === id)?.name ?? id

  return (
    <div className="settings-section settings-mcp-export">
      <h1 className="settings-heading">MCP 导出</h1>
      <div className="settings-meta">
        <p>
          将白泽工具目录以 Streamable HTTP MCP Server 暴露给外部 Agent（如 Cursor）。与「
          <Link to="/settings/mcp" className="settings-link">
            MCP
          </Link>
          」客户端注册页分开；工具导出策略在{' '}
          <Link to="/settings/tools" className="settings-link">
            Tools
          </Link>{' '}
          中配置。鉴权使用专用导出 Key（非 Gate）。
        </p>
      </div>

      {loading && <p className="settings-muted">加载中…</p>}
      {!loading && error && <p className="settings-error">{error}</p>}
      {!loading && status && <p className="settings-muted">{status}</p>}

      {!loading && settings && (
        <section className="settings-form">
          <h2 className="settings-subheading">端点</h2>
          <p className="settings-meta">
            状态：{settings.enabled ? '已启用' : '已关闭'}
            {!settings.enabled ? '（进程配置 mcp_export.enabled=false）' : null}
          </p>
          <label className="settings-field">
            <span className="settings-field-label">endpoint</span>
            <input className="settings-input" value={endpointUrl} readOnly />
          </label>
          <div className="settings-toolbar">
            <button type="button" className="btn ghost sm" onClick={() => void onCopyEndpoint()}>
              复制 endpoint
            </button>
          </div>
          <pre className="settings-muted">{`{
  "mcpServers": {
    "baize-export": {
      "url": "${endpointUrl || 'https://<host>/v0/mcp/export'}",
      "headers": {
        "Authorization": "Bearer <导出 Key>"
      }
    }
  }
}`}</pre>
        </section>
      )}

      {!loading && (
        <>
          <section className="settings-form">
            <h2 className="settings-subheading">导出身份</h2>
            <p className="settings-meta">
              每把导出 Key 必须绑定一个身份；调用时注入 headers / scheme，供 require_login 类工具使用。
            </p>
            {identities.length === 0 && (
              <p className="settings-empty">尚未创建导出身份。</p>
            )}
            {identities.length > 0 && (
              <ul className="settings-list">
                {identities.map((identity) => (
                  <li key={identity.id} className="settings-list-item">
                    {editingId === identity.id ? (
                      <form className="settings-form" onSubmit={(e) => void onSaveEdit(e)}>
                        <label className="settings-field">
                          <span className="settings-field-label">名称</span>
                          <input
                            className="settings-input"
                            value={editForm.name}
                            onChange={(e) => setEditForm((f) => ({ ...f, name: e.target.value }))}
                            disabled={busy}
                            required
                          />
                        </label>
                        <label className="settings-field">
                          <span className="settings-field-label">scheme（可选）</span>
                          <input
                            className="settings-input"
                            value={editForm.scheme}
                            onChange={(e) => setEditForm((f) => ({ ...f, scheme: e.target.value }))}
                            disabled={busy}
                            placeholder="Bearer"
                          />
                        </label>
                        <label className="settings-field">
                          <span className="settings-field-label">headers（每行 KEY=VALUE）</span>
                          <textarea
                            className="settings-textarea"
                            value={editForm.headersText}
                            onChange={(e) =>
                              setEditForm((f) => ({ ...f, headersText: e.target.value }))
                            }
                            disabled={busy}
                            rows={3}
                            placeholder="Authorization=Bearer ${TOKEN}"
                          />
                        </label>
                        <div className="settings-toolbar">
                          <button type="submit" className="btn primary sm" disabled={busy}>
                            保存
                          </button>
                          <button
                            type="button"
                            className="btn ghost sm"
                            disabled={busy}
                            onClick={() => {
                              setEditingId(null)
                              setEditForm(EMPTY_IDENTITY_FORM)
                            }}
                          >
                            取消
                          </button>
                        </div>
                      </form>
                    ) : (
                      <>
                        <span className="settings-tool-line">
                          <span className="settings-tool-title">{identity.name}</span>
                          <span className="settings-muted"> · {identity.id}</span>
                          {identity.scheme ? (
                            <span className="settings-muted"> · scheme={identity.scheme}</span>
                          ) : null}
                        </span>
                        {identity.headers && Object.keys(identity.headers).length > 0 ? (
                          <pre className="settings-muted">
                            {formatKeyValueMap(identity.headers)}
                          </pre>
                        ) : (
                          <p className="settings-muted">无 headers</p>
                        )}
                        <div className="settings-toolbar">
                          <button
                            type="button"
                            className="btn ghost sm"
                            disabled={busy}
                            onClick={() => startEdit(identity)}
                          >
                            编辑
                          </button>
                          <button
                            type="button"
                            className="btn danger sm"
                            disabled={busy}
                            onClick={() => void onDeleteIdentity(identity.id, identity.name)}
                          >
                            删除
                          </button>
                        </div>
                      </>
                    )}
                  </li>
                ))}
              </ul>
            )}

            <form className="settings-form" onSubmit={(e) => void onCreateIdentity(e)}>
              <h3 className="settings-subheading">新建身份</h3>
              <label className="settings-field">
                <span className="settings-field-label">名称</span>
                <input
                  className="settings-input"
                  value={createForm.name}
                  onChange={(e) => setCreateForm((f) => ({ ...f, name: e.target.value }))}
                  disabled={busy}
                  placeholder="Ops"
                  required
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">scheme（可选）</span>
                <input
                  className="settings-input"
                  value={createForm.scheme}
                  onChange={(e) => setCreateForm((f) => ({ ...f, scheme: e.target.value }))}
                  disabled={busy}
                  placeholder="Bearer"
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">headers（每行 KEY=VALUE）</span>
                <textarea
                  className="settings-textarea"
                  value={createForm.headersText}
                  onChange={(e) => setCreateForm((f) => ({ ...f, headersText: e.target.value }))}
                  disabled={busy}
                  rows={3}
                  placeholder="X-Team=ops"
                />
              </label>
              <div className="settings-toolbar">
                <button type="submit" className="btn primary sm" disabled={busy}>
                  创建身份
                </button>
              </div>
            </form>
          </section>

          <section className="settings-form">
            <h2 className="settings-subheading">导出 Key</h2>
            <p className="settings-meta">创建时仅展示一次明文 token；列表只显示前缀。撤销后不可恢复。</p>
            {keys.length === 0 && <p className="settings-empty">尚未创建导出 Key。</p>}
            {keys.length > 0 && (
              <ul className="settings-list">
                {keys.map((key) => {
                  const active = isKeyActive(key)
                  return (
                    <li key={key.id} className="settings-list-item">
                      <span className="settings-tool-line">
                        <span className="settings-tool-title">{key.name}</span>
                        <span className="settings-muted">
                          {' '}
                          · {key.prefix}… · {identityName(key.identity_id)}
                        </span>
                        {!active ? <span className="settings-muted"> · 已撤销</span> : null}
                      </span>
                      <div className="settings-toolbar">
                        <button
                          type="button"
                          className="btn danger sm"
                          disabled={busy || !active}
                          onClick={() => void onRevokeKey(key)}
                        >
                          {active ? '撤销' : '已撤销'}
                        </button>
                      </div>
                    </li>
                  )
                })}
              </ul>
            )}

            <form className="settings-form" onSubmit={(e) => void onCreateKey(e)}>
              <h3 className="settings-subheading">新建 Key</h3>
              <label className="settings-field">
                <span className="settings-field-label">名称</span>
                <input
                  className="settings-input"
                  value={keyName}
                  onChange={(e) => setKeyName(e.target.value)}
                  disabled={busy || identities.length === 0}
                  placeholder="cursor-dev"
                  required
                />
              </label>
              <label className="settings-field">
                <span className="settings-field-label">绑定身份</span>
                <select
                  className="settings-select"
                  value={keyIdentityId}
                  onChange={(e) => setKeyIdentityId(e.target.value)}
                  disabled={busy || identities.length === 0}
                >
                  {identities.length === 0 ? (
                    <option value="">请先创建身份</option>
                  ) : (
                    identities.map((i) => (
                      <option key={i.id} value={i.id}>
                        {i.name} ({i.id})
                      </option>
                    ))
                  )}
                </select>
              </label>
              <div className="settings-toolbar">
                <button
                  type="submit"
                  className="btn primary sm"
                  disabled={busy || identities.length === 0}
                >
                  创建 Key
                </button>
              </div>
            </form>
          </section>
        </>
      )}

      {tokenModal && (
        <div className="settings-drawer-backdrop" onClick={() => setTokenModal(null)}>
          <aside
            className="settings-drawer"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-label="新导出 Key"
          >
            <div className="settings-drawer-head">
              <h2 className="settings-subheading">Key {tokenModal.name} 明文 token</h2>
              <button type="button" className="btn ghost sm" onClick={() => setTokenModal(null)}>
                关闭
              </button>
            </div>
            <p className="settings-meta">仅展示一次，请立即复制到 MCP 客户端配置。</p>
            <pre className="settings-muted">{tokenModal.token}</pre>
            <div className="settings-toolbar">
              <button
                type="button"
                className="btn primary sm"
                onClick={() => {
                  void navigator.clipboard.writeText(tokenModal.token).then(
                    () => setStatus('已复制导出 Key'),
                    (err) => setError(`复制失败：${apiErrorMessage(err)}`),
                  )
                }}
              >
                复制 token
              </button>
            </div>
          </aside>
        </div>
      )}
    </div>
  )
}
