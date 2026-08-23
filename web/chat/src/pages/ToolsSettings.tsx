import { type FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import {
  createConnectorTool,
  deleteConnectorTool,
  getConnector,
  listTools,
  patchTool,
  putConnector,
  type ConnectorInfo,
  type ToolInfo,
} from '../api'
import { canDeleteCatalogTool, groupToolsTree, pathPrefixGroup, toolMatchesQuery } from '../toolCatalog'

const HTTP_METHODS = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE']

interface AddFormState {
  name: string
  method: string
  path: string
  title: string
  description: string
  schema: string
}

const EMPTY_FORM: AddFormState = {
  name: '',
  method: 'GET',
  path: '',
  title: '',
  description: '',
  schema: '{}',
}

function errorMessage(err: unknown): string {
  return err instanceof Error ? err.message : String(err)
}

function toolRowKey(t: ToolInfo): string {
  return `${t.connector_id}:${t.name}`
}

export function prefixExpandKey(connectorId: string, prefix: string): string {
  return `${connectorId}::${prefix}`
}

function formatMethodPath(t: ToolInfo): string {
  const method = (t.method ?? '').toUpperCase()
  const path = t.path?.trim()
  if (path) return method ? `${method} ${path}` : path
  return method
}

function isToolEnabled(t: ToolInfo): boolean {
  return t.enabled ?? true
}

function enabledCount(rows: ToolInfo[]): number {
  return rows.filter(isToolEnabled).length
}

function flattenGroup(prefixes: { tools: ToolInfo[] }[]): ToolInfo[] {
  return prefixes.flatMap((p) => p.tools)
}

function openApiConnectorIds(tools: ToolInfo[]): string[] {
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
}

function toggleKey(prev: Set<string>, key: string): Set<string> {
  const next = new Set(prev)
  if (next.has(key)) next.delete(key)
  else next.add(key)
  return next
}

function formatGroupPatchSummary(
  ok: number,
  total: number,
  failures: { name: string; reason: string }[],
): string {
  const shown = failures.slice(0, 5)
  const parts = [`已更新 ${ok}/${total}`, ...shown.map((f) => `${f.name}：${f.reason}`)]
  if (failures.length > 5) {
    parts.push(`其余 ${failures.length - 5} 条省略`)
  }
  return parts.join('；')
}

export function expandKeysForTool(t: ToolInfo): { connectorId: string; prefixKey: string } {
  const connectorId = t.connector_id || ''
  return {
    connectorId,
    prefixKey: prefixExpandKey(connectorId, pathPrefixGroup(t.path)),
  }
}

export function insertToolSorted(list: ToolInfo[], created: ToolInfo): ToolInfo[] {
  return [...list, created].sort((a, b) => a.name.localeCompare(b.name))
}

function addExpandKey(prev: Set<string>, key: string): Set<string> {
  const next = new Set(prev)
  next.add(key)
  return next
}

export function defaultExpandedSets(tools: ToolInfo[]): {
  connectors: Set<string>
  prefixes: Set<string>
} {
  const tree = groupToolsTree(tools)
  if (tree.length !== 1) {
    return { connectors: new Set(), prefixes: new Set() }
  }
  const group = tree[0]
  return {
    connectors: new Set([group.connectorId]),
    prefixes: new Set(group.prefixes.map((p) => prefixExpandKey(group.connectorId, p.prefix))),
  }
}

export function ToolsSettings() {
  const [tools, setTools] = useState<ToolInfo[] | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [expandedConnectors, setExpandedConnectors] = useState<Set<string>>(new Set())
  const [expandedPrefixes, setExpandedPrefixes] = useState<Set<string>>(new Set())
  const [toggling, setToggling] = useState<string | null>(null)
  const [deleting, setDeleting] = useState<string | null>(null)
  const [groupBusy, setGroupBusy] = useState<string | null>(null)
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [draftTitle, setDraftTitle] = useState('')
  const [draftDescription, setDraftDescription] = useState('')
  const [savingCopy, setSavingCopy] = useState(false)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [drawerError, setDrawerError] = useState<string | null>(null)
  const [form, setForm] = useState<AddFormState>(EMPTY_FORM)
  const [formConnectorId, setFormConnectorId] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [connectorMeta, setConnectorMeta] = useState<Record<string, ConnectorInfo>>({})
  const [callbackDrafts, setCallbackDrafts] = useState<Record<string, string>>({})
  const [callbackSaving, setCallbackSaving] = useState<string | null>(null)
  const [callbackError, setCallbackError] = useState<string | null>(null)
  const didInitExpand = useRef(false)
  const prevSearchRef = useRef(false)

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
          setError(errorMessage(err))
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [])

  const openConnectorIds = useMemo(() => (tools == null ? [] : openApiConnectorIds(tools)), [tools])
  const catalogConnectorIds = useMemo(() => {
    if (tools == null) return [] as string[]
    const seen = new Set<string>()
    for (const t of tools) {
      if (t.connector_id) seen.add(t.connector_id)
    }
    return [...seen]
  }, [tools])
  const searchActive = query.trim() !== ''

  useEffect(() => {
    let cancelled = false
    void (async () => {
      for (const id of catalogConnectorIds) {
        try {
          const c = await getConnector(id)
          if (!cancelled) {
            setConnectorMeta((prev) => ({ ...prev, [id]: c }))
            setCallbackDrafts((prev) => ({ ...prev, [id]: c.execution_callback_url ?? '' }))
          }
        } catch {
          /* per-connector load failure is non-fatal */
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [catalogConnectorIds])

  useEffect(() => {
    if (tools == null) return
    const wasSearch = prevSearchRef.current
    prevSearchRef.current = searchActive
    if (!didInitExpand.current) {
      didInitExpand.current = true
      if (!searchActive) {
        const defaults = defaultExpandedSets(tools)
        setExpandedConnectors(defaults.connectors)
        setExpandedPrefixes(defaults.prefixes)
      }
      return
    }
    if (wasSearch && !searchActive) {
      const defaults = defaultExpandedSets(tools)
      setExpandedConnectors(defaults.connectors)
      setExpandedPrefixes(defaults.prefixes)
    }
  }, [tools, searchActive])

  useEffect(() => {
    if (openConnectorIds.length > 0 && !openConnectorIds.includes(formConnectorId)) {
      setFormConnectorId(openConnectorIds[0])
    }
    if (openConnectorIds.length === 0 && formConnectorId !== '') {
      setFormConnectorId('')
    }
  }, [openConnectorIds, formConnectorId])

  const visible = useMemo(() => {
    if (tools == null) return [] as ToolInfo[]
    return tools.filter((t) => toolMatchesQuery(t, query))
  }, [tools, query])

  const tree = useMemo(() => groupToolsTree(visible), [visible])
  const showAdd = openConnectorIds.length > 0
  const loadFailed = tools === null && error !== null
  const rowBusy = toggling !== null || deleting !== null || groupBusy !== null || savingCopy

  const mergeTools = (updated: ToolInfo[]) => {
    setTools((prev) => {
      if (prev == null) return prev
      const byName = new Map(updated.map((t) => [t.name, t]))
      return prev.map((t) => byName.get(t.name) ?? t)
    })
  }

  const isConnectorOpen = (id: string) => searchActive || expandedConnectors.has(id)
  const isPrefixOpen = (connectorId: string, prefix: string) =>
    searchActive || expandedPrefixes.has(prefixExpandKey(connectorId, prefix))

  const onRequireLoginChange = async (name: string, requireLogin: boolean) => {
    setToggling(name)
    try {
      const updated = await patchTool(name, { require_login: requireLogin })
      mergeTools([updated])
      setError(null)
    } catch (err) {
      setError(`${name}：${errorMessage(err)}`)
    } finally {
      setToggling(null)
    }
  }

  const onEnabledChange = async (name: string, enabled: boolean) => {
    setToggling(name)
    try {
      const updated = await patchTool(name, { enabled })
      mergeTools([updated])
      setError(null)
    } catch (err) {
      setError(`${name}：${errorMessage(err)}`)
    } finally {
      setToggling(null)
    }
  }

  const onDelete = async (t: ToolInfo) => {
    const key = toolRowKey(t)
    setDeleting(key)
    try {
      await deleteConnectorTool(t.connector_id, t.name)
      setTools((prev) =>
        prev == null ? prev : prev.filter((row) => row.name !== t.name || row.connector_id !== t.connector_id),
      )
      if (editingKey === key) setEditingKey(null)
      setError(null)
    } catch (err) {
      setError(`${t.name}：${errorMessage(err)}`)
    } finally {
      setDeleting(null)
    }
  }

  const supportsExecutionCallback = (c: ConnectorInfo | undefined) =>
    c?.type === 'openapi' || c?.type === 'http'

  const saveCallbackURL = async (connectorId: string) => {
    const meta = connectorMeta[connectorId]
    if (!meta) {
      setCallbackError(`${connectorId}：尚未加载 Connector 详情`)
      return
    }
    setCallbackSaving(connectorId)
    setCallbackError(null)
    try {
      const updated = await putConnector(connectorId, {
        type: meta.type,
        spec: meta.spec,
        base_url: meta.base_url,
        execution_callback_url: (callbackDrafts[connectorId] ?? '').trim(),
        auth: meta.auth,
        require_approval: meta.require_approval,
        require_login: meta.require_login,
        mcp: meta.mcp,
      })
      setConnectorMeta((prev) => ({ ...prev, [connectorId]: updated }))
      setCallbackDrafts((prev) => ({ ...prev, [connectorId]: updated.execution_callback_url ?? '' }))
    } catch (err) {
      setCallbackError(`${connectorId}：${errorMessage(err)}`)
    } finally {
      setCallbackSaving(null)
    }
  }

  const onGroupEnabled = async (groupKey: string, rows: ToolInfo[], enabled: boolean) => {
    const names = rows.map((t) => t.name)
    if (names.length === 0) return
    setGroupBusy(groupKey)
    try {
      const results = await Promise.allSettled(names.map((n) => patchTool(n, { enabled })))
      const succeeded: ToolInfo[] = []
      const failures: { name: string; reason: string }[] = []
      results.forEach((r, i) => {
        if (r.status === 'fulfilled') {
          succeeded.push(r.value)
          return
        }
        failures.push({ name: names[i], reason: errorMessage(r.reason) })
      })
      if (succeeded.length > 0) mergeTools(succeeded)
      if (failures.length > 0) {
        setError(formatGroupPatchSummary(succeeded.length, names.length, failures))
      } else {
        setError(null)
      }
    } finally {
      setGroupBusy(null)
    }
  }

  const startEdit = (t: ToolInfo) => {
    setEditingKey(toolRowKey(t))
    setDraftTitle(t.title ?? '')
    setDraftDescription(t.description ?? '')
  }

  const onSaveCopy = async (t: ToolInfo) => {
    setSavingCopy(true)
    try {
      const updated = await patchTool(t.name, { title: draftTitle, description: draftDescription })
      mergeTools([updated])
      setEditingKey(null)
      setError(null)
    } catch (err) {
      setError(`${t.name}：${errorMessage(err)}`)
    } finally {
      setSavingCopy(false)
    }
  }

  const onAddSubmit = async (e: FormEvent) => {
    e.preventDefault()
    if (!formConnectorId) {
      setDrawerError('未选择 Connector')
      return
    }
    let schema: Record<string, unknown> = {}
    try {
      schema = form.schema.trim() === '' ? {} : (JSON.parse(form.schema) as Record<string, unknown>)
    } catch {
      setDrawerError('input_schema 不是合法 JSON')
      return
    }
    setSubmitting(true)
    try {
      const created = await createConnectorTool(formConnectorId, {
        name: form.name.trim(),
        method: form.method,
        path: form.path.trim(),
        title: form.title.trim() || undefined,
        description: form.description.trim() || undefined,
        input_schema: schema,
      })
      const keys = expandKeysForTool(created)
      setTools((prev) => (prev == null ? prev : insertToolSorted(prev, created)))
      setExpandedConnectors((prev) => addExpandKey(prev, keys.connectorId))
      setExpandedPrefixes((prev) => addExpandKey(prev, keys.prefixKey))
      setForm(EMPTY_FORM)
      setDrawerError(null)
      setDrawerOpen(false)
      setError(null)
    } catch (err) {
      setDrawerError(errorMessage(err))
    } finally {
      setSubmitting(false)
    }
  }

  const closeDrawer = () => {
    if (submitting) return
    setDrawerOpen(false)
  }

  const renderGroupButtons = (groupKey: string, rows: ToolInfo[]) => {
    const lockGroups = groupBusy !== null || toggling !== null || savingCopy
    return (
      <span className="settings-group-actions" onClick={(e) => e.stopPropagation()}>
        <button
          type="button"
          className="btn ghost sm"
          disabled={lockGroups}
          onClick={() => {
            void onGroupEnabled(groupKey, rows, true)
          }}
        >
          全部启用
        </button>
        <button
          type="button"
          className="btn ghost sm"
          disabled={lockGroups}
          onClick={() => {
            void onGroupEnabled(groupKey, rows, false)
          }}
        >
          全部停用
        </button>
      </span>
    )
  }

  const renderTool = (t: ToolInfo) => {
    const key = toolRowKey(t)
    const canDelete = canDeleteCatalogTool(t.source ?? '')
    const isDeleting = deleting === key
    const isEditing = editingKey === key
    const methodPath = formatMethodPath(t)
    const schemaText = JSON.stringify(t.input_schema ?? {}, null, 2)
    return (
      <li key={key} className="settings-tool-row">
        <div className="settings-list-item">
          <span className="settings-tool-line">
            <span className="settings-tool-title">{t.title || t.name}</span>
            {methodPath !== '' && <span className="settings-tool-sub">{methodPath}</span>}
            {t.description ? <span className="settings-tool-desc">{t.description}</span> : null}
          </span>
          <span className="settings-tool-actions">
            {t.require_approval && <span className="settings-badge">需审批</span>}
            <label className="settings-login-toggle">
              <input
                type="checkbox"
                checked={isToolEnabled(t)}
                disabled={rowBusy}
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
                disabled={rowBusy}
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
                disabled={isDeleting || rowBusy}
                onClick={() => {
                  void onDelete(t)
                }}
              >
                {isDeleting ? '删除中…' : '删除'}
              </button>
            )}
            <button
              type="button"
              className="btn ghost sm"
              disabled={savingCopy || (rowBusy && !isEditing)}
              onClick={() => {
                if (isEditing) {
                  setEditingKey(null)
                  return
                }
                startEdit(t)
              }}
            >
              {isEditing ? '收起' : '编辑文案'}
            </button>
          </span>
        </div>
        {isEditing && (
          <div className="settings-tool-edit">
            <label className="settings-field">
              <span className="settings-field-label">显示名</span>
              <input
                className="settings-input"
                value={draftTitle}
                onChange={(e) => setDraftTitle(e.target.value)}
                disabled={savingCopy}
              />
            </label>
            <label className="settings-field">
              <span className="settings-field-label">说明</span>
              <textarea
                className="settings-textarea"
                value={draftDescription}
                onChange={(e) => setDraftDescription(e.target.value)}
                disabled={savingCopy}
                rows={3}
              />
            </label>
            <p className="settings-muted">
              方法 / 路径：{methodPath || '—'}
            </p>
            <pre className="settings-tool-schema">{schemaText}</pre>
            <div className="settings-tool-edit-actions">
              <button
                type="button"
                className="btn primary sm"
                disabled={savingCopy}
                onClick={() => {
                  void onSaveCopy(t)
                }}
              >
                {savingCopy ? '保存中…' : '保存'}
              </button>
            </div>
          </div>
        )}
      </li>
    )
  }

  return (
    <div className="settings-section settings-tools">
      <h1 className="settings-heading">Tools</h1>
      {loadFailed && <p className="settings-error">无法加载 Tools：{error}</p>}
      {!loadFailed && error && <p className="settings-error">{error}</p>}
      {tools === null && !loadFailed && <p className="settings-muted">加载中…</p>}
      {tools !== null && (
        <>
          <div className="settings-toolbar">
            <input
              className="settings-input"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索"
              aria-label="搜索工具"
            />
            {showAdd && (
              <button
                type="button"
                className="btn primary sm"
                onClick={() => {
                  setDrawerError(null)
                  setDrawerOpen(true)
                }}
              >
                添加
              </button>
            )}
          </div>
          {tools.length === 0 && <p className="settings-empty">尚未注册 Connector</p>}
          {callbackError && <p className="settings-error">{callbackError}</p>}
          {tools.length > 0 && visible.length === 0 && <p className="settings-empty">无匹配</p>}
          {visible.length > 0 && (
            <div className="settings-tree">
              {tree.map((group) => {
                const connectorOpen = isConnectorOpen(group.connectorId)
                const groupRows = flattenGroup(group.prefixes)
                const connectorKey = `c:${group.connectorId}`
                return (
                  <div key={group.connectorId} className="settings-group">
                    <div className="settings-group-head">
                      <button
                        type="button"
                        className="settings-group-toggle"
                        onClick={() => setExpandedConnectors((prev) => toggleKey(prev, group.connectorId))}
                      >
                        {connectorOpen ? '▾' : '▸'} {group.connectorId || '（无 Connector）'}
                      </button>
                      <span className="settings-group-meta">
                        {groupRows.length} 个工具 · {enabledCount(groupRows)} 已启用
                      </span>
                      {renderGroupButtons(connectorKey, groupRows)}
                    </div>
                    {connectorOpen && (
                      <div className="settings-group-body">
                        {supportsExecutionCallback(connectorMeta[group.connectorId]) && (
                          <div className="settings-callback-bar settings-form" style={{ marginBottom: '0.75rem' }}>
                            <label className="settings-field">
                              <span className="settings-label">执行回调 URL（§4.3）</span>
                              <input
                                className="settings-input"
                                value={callbackDrafts[group.connectorId] ?? ''}
                                onChange={(e) =>
                                  setCallbackDrafts((prev) => ({
                                    ...prev,
                                    [group.connectorId]: e.target.value,
                                  }))
                                }
                                placeholder="https://enterprise.example/baize/execute"
                              />
                            </label>
                            <button
                              type="button"
                              className="btn sm"
                              disabled={callbackSaving === group.connectorId}
                              onClick={() => void saveCallbackURL(group.connectorId)}
                            >
                              {callbackSaving === group.connectorId ? '保存中…' : '保存回调'}
                            </button>
                            <p className="settings-hint">
                              配置后工具 invoke 走企业统一回调，不再直连 base_url 或侧车 invoke。
                            </p>
                          </div>
                        )}
                        {group.prefixes.map((prefixGroup) => {
                          const pKey = prefixExpandKey(group.connectorId, prefixGroup.prefix)
                          const prefixOpen = isPrefixOpen(group.connectorId, prefixGroup.prefix)
                          const prefixBusyKey = `p:${pKey}`
                          return (
                            <div key={pKey} className="settings-group settings-group-nested">
                              <div className="settings-group-head">
                                <button
                                  type="button"
                                  className="settings-group-toggle"
                                  onClick={() => setExpandedPrefixes((prev) => toggleKey(prev, pKey))}
                                >
                                  {prefixOpen ? '▾' : '▸'} {prefixGroup.prefix}
                                </button>
                                <span className="settings-group-meta">
                                  {prefixGroup.tools.length} 个工具 · {enabledCount(prefixGroup.tools)} 已启用
                                </span>
                                {renderGroupButtons(prefixBusyKey, prefixGroup.tools)}
                              </div>
                              {prefixOpen && (
                                <ul className="settings-list settings-tree-tools">{prefixGroup.tools.map(renderTool)}</ul>
                              )}
                            </div>
                          )
                        })}
                      </div>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </>
      )}
      {drawerOpen && (
        <div className="settings-drawer-backdrop" onClick={closeDrawer}>
          <aside
            className="settings-drawer"
            onClick={(e) => e.stopPropagation()}
            role="dialog"
            aria-label="添加工具"
          >
            <div className="settings-drawer-head">
              <h2 className="settings-subheading">添加工具</h2>
              <button type="button" className="btn ghost sm" onClick={closeDrawer} disabled={submitting}>
                关闭
              </button>
            </div>
            {drawerError && <p className="settings-error">{drawerError}</p>}
            <form className="settings-form" onSubmit={onAddSubmit}>
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
                <span className="settings-field-label">显示名（可选）</span>
                <input
                  className="settings-input"
                  value={form.title}
                  onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
                  disabled={submitting}
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
          </aside>
        </div>
      )}
    </div>
  )
}
