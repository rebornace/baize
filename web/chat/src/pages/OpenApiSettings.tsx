import { useCallback, useEffect, useState } from 'react'
import {
  deleteConnector,
  getConnector,
  listTools,
  putConnector,
  type ConnectorInfo,
  type ToolInfo,
} from '../api'
import { ToastRegion, useToast } from '../components/ui'
import { ConnectorShell, type ConnectorRowData } from '../components/settings/ConnectorShell'
import { ConnectorEditorModal, type ConnectorEditorInitial } from '../components/settings/ConnectorEditorModal'
import type { SavedConnection } from './connectorForms/types'
import { toPermissionTools } from './connectorForms/permissions'
import { CONNECTORS, connectorErrorText } from '../strings'

export function openApiConnectorIds(tools: ToolInfo[]): string[] {
  const seen = new Set<string>()
  const ordered: string[] = []
  for (const t of tools) {
    if (t.source !== 'spec' && t.source !== 'extra') continue
    if (!t.connector_id || seen.has(t.connector_id)) continue
    seen.add(t.connector_id)
    ordered.push(t.connector_id)
  }
  return ordered
}

function toRow(info: ConnectorInfo, fallbackCount: number): ConnectorRowData {
  return {
    id: info.id,
    baseUrl: info.base_url,
    toolCount: info.tools?.length ?? fallbackCount,
    loginNames: info.require_login ?? [],
    approvalNames: info.require_approval ?? [],
  }
}

export function OpenApiSettings() {
  const { toasts, push, dismiss } = useToast()
  const [rows, setRows] = useState<ConnectorRowData[]>([])
  const [connectors, setConnectors] = useState<ConnectorInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const emptyInitial: ConnectorEditorInitial = { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] }
  const [editor, setEditor] = useState<{ open: boolean; editing: boolean; initial: ConnectorEditorInitial }>({
    open: false, editing: false, initial: emptyInitial,
  })

  const load = useCallback(async () => {
    try {
      const tools = await listTools()
      const countById = new Map<string, number>()
      for (const t of tools) {
        if ((t.source === 'spec' || t.source === 'extra') && t.connector_id)
          countById.set(t.connector_id, (countById.get(t.connector_id) ?? 0) + 1)
      }
      const infos = await Promise.all(openApiConnectorIds(tools).map((id) => getConnector(id)))
      setConnectors(infos)
      setRows(infos.map((c) => toRow(c, countById.get(c.id) ?? 0)))
      setLoadError(null)
    } catch (e) {
      setLoadError(connectorErrorText(e).title)
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { void load() }, [load])

  const openCreate = () => {
    setEditor({ open: true, editing: false, initial: emptyInitial })
  }
  const openEdit = (id: string) => {
    const c = connectors.find((x) => x.id === id)
    if (!c) return
    const initial: ConnectorEditorInitial = {
      id: c.id, baseUrl: c.base_url ?? '',
      tools: toPermissionTools(c.tools),
      loginNames: c.require_login ?? [], approvalNames: c.require_approval ?? [],
      executionCallbackUrl: c.execution_callback_url ?? '',
      auth: c.auth,
    }
    setEditor({ open: true, editing: true, initial })
  }

  const handleDelete = async (id: string) => {
    await deleteConnector(id)
    push({ tone: 'success', title: `${CONNECTORS.deleted} ${id}` })
    await load()
  }

  const handleSaveInfo = async (conn: SavedConnection) => {
    if (conn.kind !== 'openapi') throw new Error('unexpected connector kind')
    const c = await putConnector(conn.id, {
      type: 'openapi',
      base_url: conn.baseUrl,
      import_format: conn.importFormat,
      spec_content: conn.spec?.content,
      spec_url: conn.spec?.url,
      execution_callback_url: conn.executionCallbackUrl,
      auth: conn.auth,
    })
    return toPermissionTools(c.tools)
  }

  return (
    <>
      <ConnectorShell kind="openapi" rows={rows} loading={loading} loadError={loadError}
        onCreate={openCreate} onEdit={openEdit} onDelete={handleDelete} />
      <ConnectorEditorModal kind="openapi" open={editor.open} editing={editor.editing} initial={editor.initial}
        onClose={() => setEditor((e) => ({ ...e, open: false }))}
        formatError={(e) => connectorErrorText(e).title}
        onSaveInfo={handleSaveInfo}
        onSavedInfo={(c) => { push({ tone: 'success', title: `${CONNECTORS.saved} ${c.id}` }); void load() }} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </>
  )
}
