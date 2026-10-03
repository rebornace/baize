import { type FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
import {
  createMCPExportIdentity,
  createMCPExportKey,
  deleteMCPExportIdentity,
  getMCPExportSettings,
  listMCPExportIdentities,
  listMCPExportKeys,
  listTools,
  patchMCPExportIdentity,
  patchTool,
  revokeMCPExportKey,
  type MCPExportIdentity,
  type MCPExportKey,
  type MCPExportSettings as MCPExportSettingsInfo,
  type ToolExportMode,
  type ToolInfo,
} from '../../api'
import { useToast } from '../../components/ui'
import { MCP_EXPORTS, mcpExportErrorText, toolErrorText } from '../../strings'
import { toolMatchesQuery } from '../../toolCatalog'
import {
  EMPTY_IDENTITY_FORM,
  identityToForm,
  mcpExportEndpointUrl,
  validateIdentityForm,
  type ConfirmState,
  type IdentityFormState,
} from './mcpExportHelpers'

export function useMcpExportSettings() {
  const { toasts, push, dismiss } = useToast()
  const [settings, setSettings] = useState<MCPExportSettingsInfo | null>(null)
  const [identities, setIdentities] = useState<MCPExportIdentity[]>([])
  const [keys, setKeys] = useState<MCPExportKey[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const [createForm, setCreateForm] = useState<IdentityFormState>(EMPTY_IDENTITY_FORM)
  const [createFormError, setCreateFormError] = useState<string | null>(null)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editForm, setEditForm] = useState<IdentityFormState>(EMPTY_IDENTITY_FORM)
  const [editFormError, setEditFormError] = useState<string | null>(null)

  const [keyName, setKeyName] = useState('')
  const [keyIdentityId, setKeyIdentityId] = useState('')
  const [keyFormError, setKeyFormError] = useState<string | null>(null)
  const [tokenModal, setTokenModal] = useState<{ name: string; token: string } | null>(null)

  const [confirm, setConfirm] = useState<ConfirmState>(null)
  const [confirmError, setConfirmError] = useState<string | null>(null)

  const [tools, setTools] = useState<ToolInfo[] | null>(null)
  const [toolsError, setToolsError] = useState<string | null>(null)
  const [toolQuery, setToolQuery] = useState('')
  const [exportBusy, setExportBusy] = useState<string | null>(null)

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
      setError(mcpExportErrorText(err).title)
    } finally {
      setLoading(false)
    }
  }, [])

  const loadTools = useCallback(async () => {
    try {
      const list = await listTools()
      setTools(list)
      setToolsError(null)
    } catch (err) {
      setTools([])
      setToolsError(toolErrorText(err).title)
    }
  }, [])

  useEffect(() => {
    void load()
  }, [load])

  useEffect(() => {
    void loadTools()
  }, [loadTools])

  const filteredTools = useMemo(() => {
    if (tools == null) return [] as ToolInfo[]
    return tools.filter((t) => toolMatchesQuery(t, toolQuery))
  }, [tools, toolQuery])

  const onExportChange = async (name: string, exportMode: ToolExportMode) => {
    setExportBusy(name)
    try {
      const updated = await patchTool(name, { export: exportMode })
      setTools((prev) =>
        prev == null ? prev : prev.map((row) => (row.name === updated.name ? { ...row, ...updated } : row)),
      )
      push({ tone: 'success', title: MCP_EXPORTS.toastExportSaved })
    } catch (err) {
      const f = toolErrorText(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setExportBusy(null)
    }
  }

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
      push({ tone: 'success', title: MCP_EXPORTS.endpointCopied })
    } catch {
      push({ tone: 'error', title: MCP_EXPORTS.copyFailed })
    }
  }

  const onCreateIdentity = async (e: FormEvent) => {
    e.preventDefault()
    const validated = validateIdentityForm(createForm)
    if (!validated.ok) {
      setCreateFormError(validated.message)
      return
    }
    setBusy(true)
    setCreateFormError(null)
    try {
      await createMCPExportIdentity({
        name: validated.name,
        scheme: validated.scheme || undefined,
        headers: Object.keys(validated.headers).length > 0 ? validated.headers : undefined,
      })
      setCreateForm(EMPTY_IDENTITY_FORM)
      push({ tone: 'success', title: MCP_EXPORTS.savedIdentity })
      await load()
    } catch (err) {
      setCreateFormError(mcpExportErrorText(err).title)
    } finally {
      setBusy(false)
    }
  }

  const startEdit = (identity: MCPExportIdentity) => {
    setEditingId(identity.id)
    setEditForm(identityToForm(identity))
    setEditFormError(null)
  }

  const cancelEdit = () => {
    setEditingId(null)
    setEditForm(EMPTY_IDENTITY_FORM)
    setEditFormError(null)
  }

  const onSaveEdit = async (e: FormEvent) => {
    e.preventDefault()
    if (!editingId) return
    const validated = validateIdentityForm(editForm)
    if (!validated.ok) {
      setEditFormError(validated.message)
      return
    }
    setBusy(true)
    setEditFormError(null)
    try {
      await patchMCPExportIdentity(editingId, {
        name: validated.name,
        scheme: validated.scheme,
        headers: validated.headers,
      })
      setEditingId(null)
      setEditForm(EMPTY_IDENTITY_FORM)
      push({ tone: 'success', title: MCP_EXPORTS.savedIdentity })
      await load()
    } catch (err) {
      setEditFormError(mcpExportErrorText(err).title)
    } finally {
      setBusy(false)
    }
  }

  const onCreateKey = async (e: FormEvent) => {
    e.preventDefault()
    const name = keyName.trim()
    const identityId = keyIdentityId.trim()
    if (!name) {
      setKeyFormError(MCP_EXPORTS.errKeyNameRequired)
      return
    }
    if (!identityId) {
      setKeyFormError(MCP_EXPORTS.errKeyIdentityRequired)
      return
    }
    setBusy(true)
    setKeyFormError(null)
    try {
      const created = await createMCPExportKey({ name, identity_id: identityId })
      setKeyName('')
      setTokenModal({ name: created.name, token: created.token })
      await load()
    } catch (err) {
      setKeyFormError(mcpExportErrorText(err).title)
    } finally {
      setBusy(false)
    }
  }

  const runConfirm = async () => {
    if (!confirm) return
    setBusy(true)
    setConfirmError(null)
    try {
      if (confirm.kind === 'identity') {
        await deleteMCPExportIdentity(confirm.id)
        if (editingId === confirm.id) {
          setEditingId(null)
          setEditForm(EMPTY_IDENTITY_FORM)
        }
        push({ tone: 'success', title: MCP_EXPORTS.deletedIdentity(confirm.name) })
      } else {
        await revokeMCPExportKey(confirm.id)
        push({ tone: 'success', title: MCP_EXPORTS.revokedKey(confirm.name) })
      }
      setConfirm(null)
      await load()
    } catch (err) {
      // 弹窗保持打开，错误内联展示，允许重试。
      setConfirmError(mcpExportErrorText(err).title)
    } finally {
      setBusy(false)
    }
  }

  const copyToken = async () => {
    if (!tokenModal) return
    try {
      await navigator.clipboard.writeText(tokenModal.token)
      push({ tone: 'success', title: MCP_EXPORTS.tokenCopied })
    } catch {
      push({ tone: 'error', title: MCP_EXPORTS.copyFailed })
    }
  }

  const identityName = (id: string) => identities.find((i) => i.id === id)?.name ?? id

  const cancelConfirm = () => {
    if (!busy) {
      setConfirm(null)
      setConfirmError(null)
    }
  }

  return {
    toasts,
    dismiss,
    settings,
    identities,
    keys,
    loading,
    error,
    busy,
    createForm,
    setCreateForm,
    createFormError,
    editingId,
    editForm,
    setEditForm,
    editFormError,
    keyName,
    setKeyName,
    keyIdentityId,
    setKeyIdentityId,
    keyFormError,
    tokenModal,
    setTokenModal,
    confirm,
    setConfirm,
    confirmError,
    tools,
    toolsError,
    toolQuery,
    setToolQuery,
    exportBusy,
    filteredTools,
    endpointUrl,
    onCopyEndpoint,
    onExportChange,
    onCreateIdentity,
    startEdit,
    cancelEdit,
    onSaveEdit,
    onCreateKey,
    runConfirm,
    copyToken,
    identityName,
    cancelConfirm,
  }
}

export type McpExportSettingsController = ReturnType<typeof useMcpExportSettings>
