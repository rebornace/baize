import { type FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  createConnectorTool,
  deleteConnectorTool,
  listTools,
  patchTool,
  type ToolInfo,
} from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  Field,
  Input,
  Modal,
  PageHeader,
  Select,
  Textarea,
  ToastRegion,
  useToast,
} from '../components/ui'
import { canDeleteCatalogTool, groupToolsTree, pathPrefixGroup, toolMatchesQuery } from '../toolCatalog'
import { useGate } from '../gateContext'
import { TOOLS, toolErrorText } from '../strings'

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

function toolErrorLabel(err: unknown): string {
  const f = toolErrorText(err)
  return f.detail ? `${f.title} ${f.detail}` : f.title
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
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const { toasts, push, dismiss } = useToast()
  const [tools, setTools] = useState<ToolInfo[] | null>(null)
  const [loadError, setLoadError] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [expandedConnectors, setExpandedConnectors] = useState<Set<string>>(new Set())
  const [expandedPrefixes, setExpandedPrefixes] = useState<Set<string>>(new Set())
  const [toggling, setToggling] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] = useState<ToolInfo | null>(null)
  const [deleting, setDeleting] = useState(false)
  const [deleteError, setDeleteError] = useState<string | null>(null)
  const [groupBusy, setGroupBusy] = useState<string | null>(null)
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [draftTitle, setDraftTitle] = useState('')
  const [draftDescription, setDraftDescription] = useState('')
  const [savingCopy, setSavingCopy] = useState(false)
  const [addModalOpen, setAddModalOpen] = useState(false)
  const [addFormError, setAddFormError] = useState<string | null>(null)
  const [form, setForm] = useState<AddFormState>(EMPTY_FORM)
  const [formConnectorId, setFormConnectorId] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const didInitExpand = useRef(false)
  const prevSearchRef = useRef(false)

  const pushToolError = (err: unknown, prefix?: string) => {
    const f = toolErrorText(err)
    push({
      tone: 'error',
      title: prefix ? `${prefix}：${f.title}` : f.title,
      detail: f.detail,
    })
  }

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const list = await listTools()
        if (!cancelled) {
          setTools(list)
          setLoadError(null)
        }
      } catch (err) {
        if (!cancelled) {
          setTools(null)
          const f = toolErrorText(err)
          setLoadError(f.detail ? `${f.title} ${f.detail}` : f.title)
          push({ tone: 'error', title: f.title, detail: f.detail })
        }
      }
    })()
    return () => {
      cancelled = true
    }
  }, [push])

  const openConnectorIds = useMemo(() => (tools == null ? [] : openApiConnectorIds(tools)), [tools])
  const searchActive = query.trim() !== ''

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
  const loadFailed = tools === null && loadError !== null
  const rowBusy = toggling !== null || deleting || groupBusy !== null || savingCopy

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
    } catch (err) {
      pushToolError(err, name)
    } finally {
      setToggling(null)
    }
  }

  const onEnabledChange = async (name: string, enabled: boolean) => {
    setToggling(name)
    try {
      const updated = await patchTool(name, { enabled })
      mergeTools([updated])
    } catch (err) {
      pushToolError(err, name)
    } finally {
      setToggling(null)
    }
  }

  const beginDelete = (t: ToolInfo) => {
    setDeleteError(null)
    setPendingDelete(t)
  }

  const cancelDelete = () => {
    if (deleting) return
    setPendingDelete(null)
    setDeleteError(null)
  }

  const confirmDelete = async () => {
    if (!pendingDelete) return
    const t = pendingDelete
    const key = toolRowKey(t)
    setDeleting(true)
    setDeleteError(null)
    try {
      await deleteConnectorTool(t.connector_id, t.name)
      setTools((prev) =>
        prev == null ? prev : prev.filter((row) => row.name !== t.name || row.connector_id !== t.connector_id),
      )
      if (editingKey === key) setEditingKey(null)
      push({ tone: 'success', title: TOOLS.toastDeleted, detail: t.title || t.name })
      setPendingDelete(null)
    } catch (err) {
      const f = toolErrorText(err)
      setDeleteError(f.detail ? `${f.title} ${f.detail}` : f.title)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setDeleting(false)
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
        failures.push({ name: names[i], reason: toolErrorLabel(r.reason) })
      })
      if (succeeded.length > 0) mergeTools(succeeded)
      if (failures.length > 0) {
        push({
          tone: 'error',
          title: TOOLS.errGeneric,
          detail: formatGroupPatchSummary(succeeded.length, names.length, failures),
        })
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
    } catch (err) {
      pushToolError(err, t.name)
    } finally {
      setSavingCopy(false)
    }
  }

  const onAddSubmit = async (e?: FormEvent) => {
    e?.preventDefault()
    if (!formConnectorId) {
      setAddFormError(TOOLS.errNoConnector)
      return
    }
    let schema: Record<string, unknown> = {}
    try {
      schema = form.schema.trim() === '' ? {} : (JSON.parse(form.schema) as Record<string, unknown>)
    } catch {
      setAddFormError(TOOLS.errInvalidSchema)
      return
    }
    setSubmitting(true)
    setAddFormError(null)
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
      setAddFormError(null)
      setAddModalOpen(false)
      push({ tone: 'success', title: TOOLS.toastAdded, detail: created.title || created.name })
    } catch (err) {
      const f = toolErrorText(err)
      setAddFormError(f.detail ? `${f.title} ${f.detail}` : f.title)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setSubmitting(false)
    }
  }

  const closeAddModal = () => {
    if (submitting) return
    setAddModalOpen(false)
    setAddFormError(null)
  }

  const renderGroupButtons = (groupKey: string, rows: ToolInfo[]) => {
    if (readOnly) return null
    const lockGroups = groupBusy !== null || toggling !== null || savingCopy
    return (
      <span className="settings-group-actions" onClick={(e) => e.stopPropagation()}>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={lockGroups}
          onClick={() => {
            void onGroupEnabled(groupKey, rows, true)
          }}
        >
          全部启用
        </Button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          disabled={lockGroups}
          onClick={() => {
            void onGroupEnabled(groupKey, rows, false)
          }}
        >
          全部停用
        </Button>
      </span>
    )
  }

  const renderTool = (t: ToolInfo) => {
    const key = toolRowKey(t)
    const canDelete = canDeleteCatalogTool(t.source ?? '')
    const isDeleting = deleting && pendingDelete != null && toolRowKey(pendingDelete) === key
    const isEditing = editingKey === key
    const methodPath = formatMethodPath(t)
    const schemaText = JSON.stringify(t.input_schema ?? {}, null, 2)
    const enabled = isToolEnabled(t)
    return (
      <li key={key} className="settings-tool-row">
        <div className="settings-list-item">
          <span className="settings-tool-line">
            <span className="settings-tool-title">{t.title || t.name}</span>
            {t.description ? <span className="settings-tool-desc">{t.description}</span> : null}
            {(methodPath !== '' || schemaText !== '{}') && (
              <details className="settings-tool-tech">
                <summary>{TOOLS.techDetails}</summary>
                {methodPath !== '' && <p className="settings-muted">{methodPath}</p>}
                {schemaText !== '{}' && <pre className="settings-tool-schema">{schemaText}</pre>}
              </details>
            )}
          </span>
          <span className="settings-tool-actions">
            {readOnly ? (
              <>
                <Badge tone={enabled ? 'success' : 'neutral'}>
                  {enabled ? TOOLS.statusEnabled : TOOLS.statusDisabled}
                </Badge>
                {t.require_login ? <Badge tone="info">{TOOLS.requireLogin}</Badge> : null}
                {t.require_approval ? (
                  <Badge tone="warning">{TOOLS.requireApprovalBadge}</Badge>
                ) : null}
              </>
            ) : (
              <>
                {t.require_approval && (
                  <Badge tone="warning">{TOOLS.requireApprovalBadge}</Badge>
                )}
                <label className="settings-login-toggle">
                  <input
                    type="checkbox"
                    checked={enabled}
                    disabled={rowBusy}
                    onChange={(e) => {
                      void onEnabledChange(t.name, e.target.checked)
                    }}
                  />
                  {TOOLS.enable}
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
                  {TOOLS.requireLogin}
                </label>
                {canDelete && (
                  <Button
                    type="button"
                    variant="danger"
                    size="sm"
                    disabled={isDeleting || rowBusy}
                    onClick={() => beginDelete(t)}
                  >
                    {TOOLS.confirmDeleteOk}
                  </Button>
                )}
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  disabled={savingCopy || (rowBusy && !isEditing)}
                  onClick={() => {
                    if (isEditing) {
                      setEditingKey(null)
                      return
                    }
                    startEdit(t)
                  }}
                >
                  {isEditing ? '收起' : TOOLS.editCopy}
                </Button>
              </>
            )}
          </span>
        </div>
        {isEditing && !readOnly && (
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
            <div className="settings-tool-edit-actions">
              <Button
                type="button"
                variant="primary"
                size="sm"
                disabled={savingCopy}
                onClick={() => {
                  void onSaveCopy(t)
                }}
              >
                {savingCopy ? '保存中…' : '保存'}
              </Button>
            </div>
          </div>
        )}
      </li>
    )
  }

  const headerActions =
    tools === null ? undefined : (
      <div className="settings-toolbar">
        <input
          className="settings-input"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          placeholder="搜索"
          aria-label="搜索工具"
        />
        {showAdd && !readOnly && (
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={() => {
              setAddFormError(null)
              setAddModalOpen(true)
            }}
          >
            {TOOLS.addTool}
          </Button>
        )}
      </div>
    )

  return (
    <div className="settings-section settings-tools">
      <PageHeader title={TOOLS.title} description={TOOLS.description} actions={headerActions} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
      {loadFailed && <p className="settings-error">{loadError}</p>}
      {tools === null && !loadFailed && <p className="settings-muted">加载中…</p>}
      {tools !== null && (
        <>
          {tools.length === 0 && (
            <p className="settings-empty">
              尚未注册 Connector。{' '}
              {!readOnly && (
                <Link to="/settings/openapi" className="settings-link">
                  去 OpenAPI 设置注册
                </Link>
              )}
            </p>
          )}
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
      {!readOnly && (
        <Modal
          open={addModalOpen}
          title={TOOLS.addModalTitle}
          onClose={submitting ? undefined : closeAddModal}
          footer={
            <>
              <Button type="button" variant="ghost" disabled={submitting} onClick={closeAddModal}>
                {TOOLS.cancel}
              </Button>
              <Button type="button" variant="primary" disabled={submitting} onClick={() => void onAddSubmit()}>
                {submitting ? '提交中…' : TOOLS.addTool}
              </Button>
            </>
          }
        >
          <p className="settings-hint">
            此处仅添加单条 extra 工具；批量导入请用{' '}
            <Link to="/settings/openapi" className="settings-link">
              OpenAPI 设置
            </Link>
            上传接口文档。
          </p>
          <form
            className="settings-form"
            onSubmit={(e) => {
              void onAddSubmit(e)
            }}
          >
            {openConnectorIds.length > 0 && (
              <Field label={TOOLS.fieldConnector} required>
                <Select
                  value={formConnectorId}
                  onChange={(e) => setFormConnectorId(e.target.value)}
                  disabled={submitting || openConnectorIds.length === 1}
                >
                  {openConnectorIds.map((id) => (
                    <option key={id} value={id}>
                      {id}
                    </option>
                  ))}
                </Select>
              </Field>
            )}
            <Field label={TOOLS.fieldName} required>
              <Input
                value={form.name}
                onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
                disabled={submitting}
                required
              />
            </Field>
            <Field label={TOOLS.fieldMethod} required>
              <Select
                value={form.method}
                onChange={(e) => setForm((f) => ({ ...f, method: e.target.value }))}
                disabled={submitting}
              >
                {HTTP_METHODS.map((m) => (
                  <option key={m} value={m}>
                    {m}
                  </option>
                ))}
              </Select>
            </Field>
            <Field label={TOOLS.fieldPath} required>
              <Input
                value={form.path}
                onChange={(e) => setForm((f) => ({ ...f, path: e.target.value }))}
                disabled={submitting}
                required
                placeholder="/items/{id}"
              />
            </Field>
            <Field label={TOOLS.fieldTitle}>
              <Input
                value={form.title}
                onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
                disabled={submitting}
              />
            </Field>
            <Field label={TOOLS.fieldDescription}>
              <Input
                value={form.description}
                onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
                disabled={submitting}
              />
            </Field>
            <Field label={TOOLS.fieldSchema} hint="JSON">
              <Textarea
                value={form.schema}
                onChange={(e) => setForm((f) => ({ ...f, schema: e.target.value }))}
                disabled={submitting}
                rows={4}
              />
            </Field>
          </form>
          {addFormError && (
            <p className="ui-inline-error" role="alert">
              {addFormError}
            </p>
          )}
        </Modal>
      )}
      {!readOnly && (
        <ConfirmDialog
          open={!!pendingDelete}
          danger
          title={TOOLS.confirmDeleteTitle}
          body={TOOLS.confirmDeleteBody}
          confirmText={TOOLS.confirmDeleteOk}
          busy={deleting}
          error={deleteError}
          onCancel={cancelDelete}
          onConfirm={() => void confirmDelete()}
        />
      )}
    </div>
  )
}
