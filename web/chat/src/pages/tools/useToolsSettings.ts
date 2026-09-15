import { type FormEvent, useEffect, useMemo, useRef, useState } from 'react'
import {
  createConnectorTool,
  deleteConnectorTool,
  listTools,
  patchTool,
  type ToolInfo,
} from '../../api'
import { useToast } from '../../components/ui'
import { useGate } from '../../gateContext'
import { groupToolsTree, toolMatchesQuery } from '../../toolCatalog'
import { TOOLS, toolErrorText } from '../../strings'
import {
  EMPTY_FORM,
  addExpandKey,
  defaultExpandedSets,
  expandKeysForTool,
  formatGroupPatchSummary,
  insertToolSorted,
  openApiConnectorIds,
  prefixExpandKey,
  toolErrorLabel,
  toolRowKey,
  type AddFormState,
} from './toolsSettingsHelpers'

export function useToolsSettings() {
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

  const onRequireApprovalChange = async (name: string, requireApproval: boolean) => {
    setToggling(name)
    try {
      const updated = await patchTool(name, { require_approval: requireApproval })
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

  const openAddModal = () => {
    setAddFormError(null)
    setAddModalOpen(true)
  }

  return {
    readOnly,
    toasts,
    dismiss,
    tools,
    loadError,
    loadFailed,
    query,
    setQuery,
    tree,
    visible,
    showAdd,
    rowBusy,
    groupBusy,
    toggling,
    savingCopy,
    editingKey,
    setEditingKey,
    draftTitle,
    setDraftTitle,
    draftDescription,
    setDraftDescription,
    pendingDelete,
    deleting,
    deleteError,
    addModalOpen,
    addFormError,
    form,
    setForm,
    formConnectorId,
    setFormConnectorId,
    openConnectorIds,
    submitting,
    isConnectorOpen,
    isPrefixOpen,
    setExpandedConnectors,
    setExpandedPrefixes,
    onEnabledChange,
    onRequireLoginChange,
    onRequireApprovalChange,
    beginDelete,
    cancelDelete,
    confirmDelete,
    onGroupEnabled,
    startEdit,
    onSaveCopy,
    onAddSubmit,
    closeAddModal,
    openAddModal,
  }
}

export type ToolsSettingsController = ReturnType<typeof useToolsSettings>
