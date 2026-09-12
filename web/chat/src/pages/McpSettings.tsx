import { useCallback, useEffect, useState } from 'react'
import {
  deleteConnector,
  getConnector,
  listTools,
  putConnector,
  type ConnectorInfo,
} from '../api'
import { ToastRegion, useToast } from '../components/ui'
import { ConnectorShell, type ConnectorRowData } from '../components/settings/ConnectorShell'
import { ConnectorEditorModal, type ConnectorEditorInitial } from '../components/settings/ConnectorEditorModal'
import { CONNECTORS, connectorErrorText } from '../strings'
import { mcpConnectorIds, mcpSummary } from './connectorForms/mcp'
import type { SavedConnection } from './connectorForms/types'

function toRow(info: ConnectorInfo, fallbackCount: number): ConnectorRowData {
  return {
    id: info.id,
    summary: mcpSummary(info.mcp),
    toolCount: info.tools?.length ?? fallbackCount,
    loginNames: [],
    approvalNames: info.require_approval ?? [],
  }
}

export function McpSettings() {
  const { toasts, push, dismiss } = useToast()
  const [rows, setRows] = useState<ConnectorRowData[]>([])
  const [connectors, setConnectors] = useState<ConnectorInfo[]>([])
  const [loading, setLoading] = useState(true)
  const [loadError, setLoadError] = useState<string | null>(null)
  const emptyInitial: ConnectorEditorInitial = { id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [] }
  const [editor, setEditor] = useState<{ open: boolean; editing: boolean; initial: ConnectorEditorInitial }>({
    open: false, editing: false, initial: emptyInitial,
  })
  // 缓存第一步的连接级负载：第二步 PUT 必须整表回传完整 mcp（后端每次 PUT 都重新探测）。
  const [savedConn, setSavedConn] = useState<SavedConnection | null>(null)

  const load = useCallback(async () => {
    try {
      const tools = await listTools()
      const countById = new Map<string, number>()
      for (const t of tools) {
        if (t.source === 'mcp' && t.connector_id) countById.set(t.connector_id, (countById.get(t.connector_id) ?? 0) + 1)
      }
      const infos = await Promise.all(mcpConnectorIds(tools).map((id) => getConnector(id)))
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
    setSavedConn(null)
    setEditor({ open: true, editing: false, initial: emptyInitial })
  }
  const openEdit = (id: string) => {
    const c = connectors.find((x) => x.id === id)
    // 编辑态 MCP 连接器理应必有 mcp；缺失属数据异常，不打开编辑器，也不 fallback 假配置。
    if (!c || !c.mcp) return
    // 预构造连接级负载，使「直接进入工具权限」也能整表回传。
    const conn: SavedConnection = { kind: 'mcp', id: c.id, mcp: c.mcp }
    setSavedConn(conn)
    setEditor({
      open: true, editing: true,
      initial: {
        id: c.id, baseUrl: '',
        tools: (c.tools ?? []).map((t) => ({ name: t.name })),
        loginNames: [], approvalNames: c.require_approval ?? [], mcp: c.mcp,
      },
    })
  }

  const handleDelete = async (id: string) => {
    await deleteConnector(id)
    push({ tone: 'success', title: `${CONNECTORS.deleted} ${id}` })
    await load()
  }

  const handleSaveInfo = async (conn: SavedConnection) => {
    // 判别收窄：本页 kind 恒为 mcp，非 mcp 正常不可达。
    if (conn.kind !== 'mcp') return []
    const c = await putConnector(conn.id, { type: 'mcp', mcp: conn.mcp })
    setSavedConn(conn)
    return (c.tools ?? []).map((t) => ({ name: t.name }))
  }
  const handleSavePermissions = async (_id: string, _login: string[], approvalNames: string[]) => {
    // 连接级字段整表回传（后端每次 PUT 都重新探测）；MCP 无 login 位，不带 require_login。
    // 缓存缺失属接线错误：显式抛出，由组件 finish 的 catch 提示用户，避免静默丢权限。
    if (!savedConn || savedConn.kind !== 'mcp') {
      throw new Error('saved mcp connection missing before permissions save')
    }
    await putConnector(savedConn.id, { type: 'mcp', mcp: savedConn.mcp, require_approval: approvalNames })
    push({ tone: 'success', title: `${CONNECTORS.saved} ${savedConn.id}` })
    await load()
  }

  return (
    <>
      <ConnectorShell kind="mcp" rows={rows} loading={loading} loadError={loadError}
        onCreate={openCreate} onEdit={openEdit} onDelete={handleDelete} />
      <ConnectorEditorModal kind="mcp" open={editor.open} editing={editor.editing} initial={editor.initial}
        onClose={() => setEditor((e) => ({ ...e, open: false }))}
        formatError={(e) => connectorErrorText(e).title}
        onSaveInfo={handleSaveInfo} onSavePermissions={handleSavePermissions}
        onSavedInfo={(c) => { setSavedConn(c); push({ tone: 'success', title: `${CONNECTORS.saved} ${c.id}` }); void load() }} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </>
  )
}
