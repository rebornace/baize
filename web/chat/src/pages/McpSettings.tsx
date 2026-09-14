import { useCallback, useEffect, useRef, useState } from 'react'
import {
  ApiError,
  deleteConnector,
  disconnectMcpOAuth,
  listConnectors,
  patchRuntimeSettings,
  putConnector,
  startMcpOAuth,
  type ConnectorInfo,
} from '../api'
import { ToastRegion, useToast, Modal, Button, Field, Input } from '../components/ui'
import { ConnectorShell, type ConnectorRowData } from '../components/settings/ConnectorShell'
import { ConnectorEditorModal, type ConnectorEditorInitial } from '../components/settings/ConnectorEditorModal'
import { CONNECTORS, connectorErrorText } from '../strings'
import { mcpSummary } from './connectorForms/mcp'
import { toPermissionTools } from './connectorForms/permissions'
import type { SavedConnection } from './connectorForms/types'

function toRow(info: ConnectorInfo): ConnectorRowData {
  const http = info.mcp?.transport === 'http'
  return {
    id: info.id,
    summary: mcpSummary(info.mcp),
    toolCount: info.tools?.length ?? 0,
    loginNames: [],
    approvalNames: info.require_approval ?? [],
    supportsOAuth: http,
    oauthStatus: http ? (info.mcp?.oauth?.status ?? '') : undefined,
  }
}

const OAUTH_STATUS_POLL_MS = 2500
const OAUTH_STATUS_POLL_MAX_MS = 90_000

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
  const oauthPollStopRef = useRef<(() => void) | null>(null)
  const [publicBasePrompt, setPublicBasePrompt] = useState<{ connectorId: string; value: string } | null>(null)
  const [publicBaseBusy, setPublicBaseBusy] = useState(false)

  const load = useCallback(async () => {
    try {
      // List connectors from store (not tools): OAuth MCP may save with 0 tools
      // until「去授权」completes, and must still appear on this page.
      const infos = await listConnectors('mcp')
      setConnectors(infos)
      setRows(infos.map((c) => toRow(c)))
      setLoadError(null)
    } catch (e) {
      setLoadError(connectorErrorText(e).title)
    } finally {
      setLoading(false)
    }
  }, [])
  useEffect(() => { void load() }, [load])
  useEffect(() => () => { oauthPollStopRef.current?.() }, [])

  const startOAuthStatusPoll = useCallback(() => {
    oauthPollStopRef.current?.()
    const started = Date.now()
    const tick = () => {
      if (Date.now() - started > OAUTH_STATUS_POLL_MAX_MS) {
        stop()
        return
      }
      void load()
    }
    const onFocus = () => { tick() }
    const onVis = () => {
      if (document.visibilityState === 'visible') tick()
    }
    const intervalId = window.setInterval(tick, OAUTH_STATUS_POLL_MS)
    const stop = () => {
      window.clearInterval(intervalId)
      window.removeEventListener('focus', onFocus)
      document.removeEventListener('visibilitychange', onVis)
      if (oauthPollStopRef.current === stop) oauthPollStopRef.current = null
    }
    window.addEventListener('focus', onFocus)
    document.addEventListener('visibilitychange', onVis)
    oauthPollStopRef.current = stop
    window.setTimeout(stop, OAUTH_STATUS_POLL_MAX_MS)
  }, [load])

  const openCreate = () => {
    setEditor({ open: true, editing: false, initial: emptyInitial })
  }
  const openEdit = (id: string) => {
    const c = connectors.find((x) => x.id === id)
    // 编辑态 MCP 连接器理应必有 mcp；缺失属数据异常，不打开编辑器，也不 fallback 假配置。
    if (!c || !c.mcp) return
    setEditor({
      open: true, editing: true,
      initial: {
        id: c.id, baseUrl: '',
        tools: toPermissionTools(c.tools),
        loginNames: [], approvalNames: c.require_approval ?? [], mcp: c.mcp,
      },
    })
  }

  const handleDelete = async (id: string) => {
    await deleteConnector(id)
    push({ tone: 'success', title: `${CONNECTORS.deleted} ${id}` })
    await load()
  }

  const runAuthorize = async (id: string) => {
    const { authorization_url: url } = await startMcpOAuth(id)
    window.open(url, '_blank', 'noopener,noreferrer')
    push({ tone: 'success', title: CONNECTORS.oauthAuthorizeOpened })
    startOAuthStatusPoll()
  }

  const handleAuthorize = async (id: string) => {
    try {
      await runAuthorize(id)
    } catch (e) {
      if (e instanceof ApiError && e.code === 'public_base_required') {
        setPublicBasePrompt({
          connectorId: id,
          value: typeof window !== 'undefined' ? window.location.origin : 'http://127.0.0.1:8080',
        })
        return
      }
      const err = connectorErrorText(e)
      push({ tone: 'error', title: err.title, detail: err.detail })
    }
  }

  const confirmPublicBaseAndAuthorize = async () => {
    if (!publicBasePrompt) return
    const trimmed = publicBasePrompt.value.trim()
    if (!trimmed || !/^https?:\/\//i.test(trimmed)) {
      push({ tone: 'error', title: CONNECTORS.errOAuthPublicBase })
      return
    }
    setPublicBaseBusy(true)
    try {
      await patchRuntimeSettings({ public_base_url: trimmed })
      const id = publicBasePrompt.connectorId
      setPublicBasePrompt(null)
      await runAuthorize(id)
    } catch (e) {
      const err = connectorErrorText(e)
      push({ tone: 'error', title: err.title, detail: err.detail })
    } finally {
      setPublicBaseBusy(false)
    }
  }

  const handleDisconnectOAuth = async (id: string) => {
    try {
      await disconnectMcpOAuth(id)
      push({ tone: 'success', title: CONNECTORS.oauthDisconnected })
      await load()
    } catch (e) {
      const err = connectorErrorText(e)
      push({ tone: 'error', title: err.title, detail: err.detail })
    }
  }

  const handleSaveInfo = async (conn: SavedConnection) => {
    // 判别收窄：本页 kind 恒为 mcp，非 mcp 正常不可达。
    if (conn.kind !== 'mcp') return []
    const c = await putConnector(conn.id, { type: 'mcp', mcp: conn.mcp })
    return toPermissionTools(c.tools)
  }

  return (
    <>
      <ConnectorShell kind="mcp" rows={rows} loading={loading} loadError={loadError}
        onCreate={openCreate} onEdit={openEdit} onDelete={handleDelete}
        onAuthorize={handleAuthorize} onDisconnectOAuth={handleDisconnectOAuth} />
      <ConnectorEditorModal kind="mcp" open={editor.open} editing={editor.editing} initial={editor.initial}
        onClose={() => setEditor((e) => ({ ...e, open: false }))}
        formatError={(e) => {
          const err = connectorErrorText(e)
          return err.detail ? `${err.title}\n${err.detail}` : err.title
        }}
        onSaveInfo={handleSaveInfo}
        onSavedInfo={(c) => { push({ tone: 'success', title: `${CONNECTORS.saved} ${c.id}` }); void load() }} />
      <Modal
        open={publicBasePrompt !== null}
        title={CONNECTORS.oauthPublicBaseTitle}
        onClose={() => {
          if (publicBaseBusy) return
          setPublicBasePrompt(null)
        }}
        footer={
          <>
            <Button
              type="button"
              variant="ghost"
              disabled={publicBaseBusy}
              onClick={() => setPublicBasePrompt(null)}
            >
              {CONNECTORS.oauthPublicBaseCancel}
            </Button>
            <Button
              type="button"
              variant="primary"
              disabled={publicBaseBusy}
              onClick={() => void confirmPublicBaseAndAuthorize()}
            >
              {publicBaseBusy ? CONNECTORS.oauthPublicBaseSaveRetry + '…' : CONNECTORS.oauthPublicBaseSaveRetry}
            </Button>
          </>
        }
      >
        <p className="settings-muted">{CONNECTORS.oauthPublicBaseBody}</p>
        <Field label={CONNECTORS.oauthPublicBaseTitle}>
          <Input
            type="url"
            value={publicBasePrompt?.value ?? ''}
            onChange={(e) =>
              setPublicBasePrompt((p) => (p ? { ...p, value: e.target.value } : p))
            }
            disabled={publicBaseBusy}
            placeholder="http://127.0.0.1:8080"
          />
        </Field>
      </Modal>
      <ToastRegion toasts={toasts} onDismiss={dismiss} />
    </>
  )
}
