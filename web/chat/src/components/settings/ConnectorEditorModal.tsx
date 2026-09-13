import { useEffect, useRef, useState } from 'react'
import { Button, Field, Input, Modal, Select, Textarea } from '../ui'
import { CONNECTORS, TOOLS } from '../../strings'
import { FilePickerButton } from './FilePickerButton'
import type { ConnectorAuth, ImportFormat, MCPConfig } from '../../api'
import { CaptureSettingsFields } from '../../pages/CaptureSettingsFields'
import {
  captureToDraft,
  mergeAuthWithCapture,
  type CaptureDraft,
} from '../../pages/captureForm'
import { validateConnection } from '../../pages/connectorForms/validate'
import {
  connectorToMcpForm,
  validateMcp,
  type McpFieldErrors,
  type McpFormValues,
} from '../../pages/connectorForms/mcp'
import {
  emptySelection,
  filterPermissionTools,
  selectionFromLists,
  toNameLists,
  toPermissionTools,
  toggleTool,
  type PermissionTool,
} from '../../pages/connectorForms/permissions'
import type {
  ConnectorKind,
  FieldErrors,
  PermissionSelection,
  SavedConnection,
} from '../../pages/connectorForms/types'

export interface ConnectorEditorInitial {
  id: string
  baseUrl: string
  tools: PermissionTool[]
  loginNames: string[]
  approvalNames: string[]
  mcp?: MCPConfig
  executionCallbackUrl?: string
  /** 用于回显 capture */
  auth?: ConnectorAuth
}

const EMPTY_MCP_FORM: McpFormValues = {
  id: '', transport: 'stdio', command: '', argsText: '', envText: '', url: '', headersText: '',
}

export interface ConnectorEditorModalProps {
  kind: ConnectorKind
  open: boolean
  editing: boolean
  initial: ConnectorEditorInitial
  onClose: () => void
  formatError: (e: unknown) => string
  onSaveInfo: (conn: SavedConnection) => Promise<PermissionTool[]>
  onSavePermissions: (
    id: string,
    loginNames: string[],
    approvalNames: string[],
    advanced?: { executionCallbackUrl: string; auth?: ConnectorAuth },
  ) => Promise<void>
  // 第一步连接信息真正保存成功时回调一次（失败不回调）；页面据此缓存连接级负载、
  // 立即提示并刷新列表，避免用户新建后直接跳过/关闭弹窗看不到新连接器。
  onSavedInfo?: (conn: SavedConnection) => void
}

const EMPTY_INITIAL: ConnectorEditorInitial = {
  id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [],
}

/**
 * 重建第 2 步的权限选择：保留 prev 中同名工具已有的勾选，
 * 仅对新出现的工具使用 fallback（编辑回显 / 新建全不勾）。
 */
const reselect = (tools: PermissionTool[], fallback: PermissionSelection) =>
  (prev: PermissionSelection): PermissionSelection => {
    const out: PermissionSelection = {}
    for (const t of tools) {
      out[t.name] = prev[t.name] ?? fallback[t.name] ?? { login: false, approval: false }
    }
    return out
  }

export function ConnectorEditorModal(props: ConnectorEditorModalProps) {
  const { kind, open, editing, initial = EMPTY_INITIAL } = props
  // 父级可能渲染期内联传入新的 initial 对象；重置只允许在 open 变 true 时发生一次，
  // 故用 ref 读最新值、effect 仅依赖 open（I-3）。
  const initialRef = useRef(initial)
  initialRef.current = initial
  const [step, setStep] = useState<1 | 2>(1)
  const [id, setId] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [importFormat, setImportFormat] = useState<ImportFormat>('auto')
  const [specContent, setSpecContent] = useState<string | null>(null)
  const [specFileName, setSpecFileName] = useState<string | null>(null)
  const [specUrl, setSpecUrl] = useState('')
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [formError, setFormError] = useState<string | null>(null)
  const [tools, setTools] = useState<PermissionTool[]>([])
  const [toolQuery, setToolQuery] = useState('')
  const [selection, setSelection] = useState<PermissionSelection>({})
  const [saving, setSaving] = useState(false)
  const [mcpForm, setMcpForm] = useState<McpFormValues>(EMPTY_MCP_FORM)
  const [mcpErrors, setMcpErrors] = useState<McpFieldErrors>({})
  const [executionCallbackUrl, setExecutionCallbackUrl] = useState('')
  const [captureDraft, setCaptureDraft] = useState<CaptureDraft>(() => captureToDraft(undefined))
  const isMcp = kind === 'mcp'
  const isOpenapi = kind === 'openapi'

  useEffect(() => {
    if (!open) return
    const init = initialRef.current
    setStep(1)
    setId(init.id)
    setBaseUrl(init.baseUrl)
    setImportFormat('auto')
    setSpecContent(null)
    setSpecFileName(null)
    setSpecUrl('')
    setFieldErrors({})
    setFormError(null)
    setSaving(false)
    setTools(toPermissionTools(init.tools))
    setToolQuery('')
    setSelection(selectionFromLists(init.tools, init.loginNames, init.approvalNames))
    setMcpForm(init.mcp
      ? connectorToMcpForm({ id: init.id, type: 'mcp', mcp: init.mcp })
      : { ...EMPTY_MCP_FORM })
    setMcpErrors({})
    setExecutionCallbackUrl(init.executionCallbackUrl ?? '')
    setCaptureDraft(captureToDraft(init.auth?.capture))
  }, [open])

  if (!open) return null

  const visibleTools = filterPermissionTools(tools, toolQuery)

  const title = editing
    ? { openapi: CONNECTORS.editOpenapi, plugin: CONNECTORS.editPlugin, mcp: CONNECTORS.editMcp }[kind]
    : { openapi: CONNECTORS.addOpenapi, plugin: CONNECTORS.addPlugin, mcp: CONNECTORS.addMcp }[kind]

  const onSpecFile = (file: File | undefined) => {
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      setSpecContent(typeof reader.result === 'string' ? reader.result : '')
      setSpecFileName(file.name)
      setSpecUrl('')
      setFieldErrors((prev) => ({ ...prev, spec: undefined }))
      setFormError(null)
    }
    reader.onerror = () => {
      setFormError(CONNECTORS.specReadFailed)
    }
    reader.readAsText(file)
  }

  const saveInfo = async () => {
    let conn: SavedConnection | null = null
    if (isMcp) {
      // MCP 复用顶部共享「连接编号」输入（id state），传输字段在 mcpForm。
      const result = validateMcp({ ...mcpForm, id })
      if (!result.ok) {
        setFieldErrors((p) => ({ ...p, id: result.fieldErrors.id }))
        setMcpErrors({
          command: result.fieldErrors.command,
          url: result.fieldErrors.url,
          env: result.fieldErrors.env,
          headers: result.fieldErrors.headers,
        })
        return
      }
      setFieldErrors({})
      setMcpErrors({})
      conn = { kind: 'mcp', id: result.id, mcp: result.mcp }
    } else {
      const hasSpec = specContent != null || specUrl.trim() !== ''
      const result = validateConnection({ kind, id, baseUrl, hasSpec, editing })
      if (!result.ok) {
        setFieldErrors(result.fieldErrors)
        return
      }
      setFieldErrors({})
      const advancedAuth = mergeAuthWithCapture(initialRef.current.auth, captureDraft)
      const callbackUrl = executionCallbackUrl.trim()
      if (kind === 'openapi') {
        conn = {
          kind: 'openapi',
          id: result.id,
          baseUrl: result.baseUrl,
          importFormat,
          executionCallbackUrl: callbackUrl,
          auth: advancedAuth,
          spec: hasSpec
            ? { content: specContent ?? undefined, url: specContent == null ? specUrl.trim() : undefined }
            : undefined,
        }
      } else {
        conn = {
          kind: 'plugin',
          id: result.id,
          baseUrl: result.baseUrl,
          executionCallbackUrl: callbackUrl,
          auth: advancedAuth,
        }
      }
    }
    setFormError(null)
    setSaving(true)
    try {
      const discovered = await props.onSaveInfo(conn)
      const nextTools = discovered.length > 0 ? toPermissionTools(discovered) : initial.tools
      const fallback = editing
        ? selectionFromLists(nextTools, isMcp ? [] : initial.loginNames, initial.approvalNames)
        : emptySelection(nextTools)
      setTools(nextTools)
      setToolQuery('')
      // 保留本次打开期间已勾选过的同名工具权限（I-1）
      setSelection(reselect(nextTools, fallback))
      setStep(2)
      // 第一步已真正保存（连接器已创建/更新）：通知页面缓存连接级负载、提示并刷新列表，
      // 这样用户随后直接「暂不设置」或关闭弹窗也能看到新连接器。
      props.onSavedInfo?.(conn)
    } catch (e) {
      setFormError(props.formatError(e))
    } finally {
      setSaving(false)
    }
  }

  // 编辑态不重存第一步、直接去第二步：纯导航，不触发保存成功回调
  // （第二步 PUT 所需的连接级负载由各页 openEdit 打开弹窗前预构造缓存）。
  const gotoPermissions = () => {
    const fallback = selectionFromLists(
      initial.tools,
      isMcp ? [] : initial.loginNames,
      initial.approvalNames,
    )
    setTools(toPermissionTools(initial.tools))
    setToolQuery('')
    // 保留本次打开期间已勾选过的同名工具权限（I-1）
    setSelection(reselect(initial.tools, fallback))
    setStep(2)
  }

  const removeSpecFile = () => {
    setSpecContent(null)
    setSpecFileName(null)
    setFieldErrors((prev) => ({ ...prev, spec: undefined }))
  }

  const finish = async () => {
    setSaving(true)
    try {
      const { loginNames, approvalNames } = toNameLists(selection)
      // MCP 无 login 位：恒传空 login 名单（页面据此省略 require_login）。
      // 非 MCP：即便用户跳过「保存连接」直接进权限，也要带上当前高级草稿。
      await props.onSavePermissions(
        id.trim(),
        isMcp ? [] : loginNames,
        approvalNames,
        isMcp
          ? undefined
          : {
              executionCallbackUrl: executionCallbackUrl.trim(),
              auth: mergeAuthWithCapture(initialRef.current.auth, captureDraft),
            },
      )
      props.onClose()
    } catch (e) {
      setFormError(props.formatError(e))
      setSaving(false)
    }
  }

  return (
    <Modal
      open={open}
      title={title}
      onClose={saving ? undefined : props.onClose}
      footer={
        step === 1 ? (
          <>
            <Button variant="secondary" onClick={props.onClose} disabled={saving}>{CONNECTORS.cancel}</Button>
            {editing && initial.tools.length > 0 && (
              <Button variant="secondary" onClick={gotoPermissions} disabled={saving}>
                {CONNECTORS.nextToPermissions}
              </Button>
            )}
            <Button variant="primary" onClick={() => void saveInfo()} disabled={saving}>
              {saving ? CONNECTORS.saving : CONNECTORS.save}
            </Button>
          </>
        ) : (
          <>
            <Button variant="secondary" onClick={() => setStep(1)} disabled={saving}>{CONNECTORS.back}</Button>
            {!editing && (
              <Button variant="secondary" onClick={props.onClose} disabled={saving}>{CONNECTORS.skip}</Button>
            )}
            <Button variant="primary" onClick={() => void finish()} disabled={saving}>
              {saving ? CONNECTORS.saving : CONNECTORS.finish}
            </Button>
          </>
        )
      }
    >
      {formError && <p className="ui-inline-error" role="alert">{formError}</p>}
      {step === 1 ? (
        <div className="connector-form">
          <Field label={CONNECTORS.fieldId} hint={CONNECTORS.fieldIdHint} required error={fieldErrors.id}>
            <Input value={id} disabled={editing || saving} placeholder="ticket-api"
              onChange={(e) => { setId(e.target.value); setFieldErrors((p) => ({ ...p, id: undefined })) }} />
          </Field>
          {isMcp && (
            <>
              <Field label={CONNECTORS.fieldTransport} required>
                <Select value={mcpForm.transport} disabled={saving}
                  onChange={(e) => setMcpForm((f) => ({ ...f, transport: e.target.value === 'http' ? 'http' : 'stdio' }))}>
                  <option value="stdio">{CONNECTORS.transportStdio}</option>
                  <option value="http">{CONNECTORS.transportHttp}</option>
                </Select>
              </Field>
              {mcpForm.transport === 'stdio' ? (
                <>
                  <Field label={CONNECTORS.fieldCommand} hint={CONNECTORS.fieldCommandHint} required error={mcpErrors.command}>
                    <Input value={mcpForm.command} disabled={saving} placeholder="npx"
                      onChange={(e) => {
                        setMcpForm((f) => ({ ...f, command: e.target.value }))
                        setMcpErrors((p) => ({ ...p, command: undefined }))
                      }} />
                  </Field>
                  <Field label={CONNECTORS.fieldArgs} hint={CONNECTORS.fieldArgsHint}>
                    <Textarea rows={3} value={mcpForm.argsText} disabled={saving} placeholder="@bytebase/dbhub"
                      onChange={(e) => setMcpForm((f) => ({ ...f, argsText: e.target.value }))} />
                  </Field>
                  <Field label={CONNECTORS.fieldEnv} hint={CONNECTORS.fieldEnvHint} error={mcpErrors.env}>
                    <Textarea rows={3} value={mcpForm.envText} disabled={saving} placeholder={'DSN=${DSN}'}
                      onChange={(e) => {
                        setMcpForm((f) => ({ ...f, envText: e.target.value }))
                        setMcpErrors((p) => ({ ...p, env: undefined }))
                      }} />
                  </Field>
                </>
              ) : (
                <>
                  <Field label={CONNECTORS.fieldUrl} required error={mcpErrors.url}>
                    <Input value={mcpForm.url} disabled={saving} placeholder="https://mcp.example.com/mcp"
                      onChange={(e) => {
                        setMcpForm((f) => ({ ...f, url: e.target.value }))
                        setMcpErrors((p) => ({ ...p, url: undefined }))
                      }} />
                  </Field>
                  <Field label={CONNECTORS.fieldHeaders} hint={CONNECTORS.fieldHeadersHint} error={mcpErrors.headers}>
                    <Textarea rows={3} value={mcpForm.headersText} disabled={saving} placeholder="Authorization=Bearer ${TOKEN}"
                      onChange={(e) => {
                        setMcpForm((f) => ({ ...f, headersText: e.target.value }))
                        setMcpErrors((p) => ({ ...p, headers: undefined }))
                      }} />
                  </Field>
                </>
              )}
            </>
          )}
          {!isMcp && (
            <Field label={CONNECTORS.fieldBaseUrl} required error={fieldErrors.baseUrl}>
              <Input value={baseUrl} disabled={saving}
                placeholder={isOpenapi ? 'https://api.example.com' : 'http://127.0.0.1:19090'}
                onChange={(e) => { setBaseUrl(e.target.value); setFieldErrors((p) => ({ ...p, baseUrl: undefined })) }} />
            </Field>
          )}
          {isOpenapi && (
            <>
              <Field label={CONNECTORS.fieldSpec} hint={CONNECTORS.fieldSpecHint} error={fieldErrors.spec}>
                <FilePickerButton
                  accept=".json,.yaml,.yml"
                  chooseLabel={CONNECTORS.chooseSpec}
                  clearLabel={CONNECTORS.specRemoveFile}
                  fileName={specFileName}
                  disabled={saving}
                  onFile={onSpecFile}
                  onClear={removeSpecFile}
                />
              </Field>
              <Field label={CONNECTORS.fieldSpecUrl}>
                <Input value={specUrl} disabled={saving || specContent != null}
                  placeholder="https://api.example.com/openapi.json"
                  onChange={(e) => {
                    const v = e.target.value
                    setSpecUrl(v)
                    // 双保险：改为填链接时清掉已选文件（I-2）
                    if (v.trim() !== '') {
                      setSpecContent(null)
                      setSpecFileName(null)
                    }
                    setFieldErrors((p) => ({ ...p, spec: undefined }))
                  }} />
              </Field>
              <Field label={CONNECTORS.fieldFormat}>
                <Select value={importFormat} disabled={saving}
                  onChange={(e) => setImportFormat(e.target.value as ImportFormat)}>
                  <option value="auto">{CONNECTORS.fmtAuto}</option>
                  <option value="openapi3">{CONNECTORS.fmtOpenapi3}</option>
                  <option value="swagger2">{CONNECTORS.fmtSwagger2}</option>
                  <option value="postman">{CONNECTORS.fmtPostman}</option>
                </Select>
              </Field>
            </>
          )}
          {!isMcp && (
            <details className="settings-advanced">
              <summary>{CONNECTORS.advanced}</summary>
              <section className="settings-advanced-block">
                <h4 className="settings-advanced-title">{CONNECTORS.executionCallbackSection}</h4>
                <Field
                  label={CONNECTORS.executionCallback}
                  hint={
                    <>
                      {CONNECTORS.executionCallbackHint}
                      <pre className="settings-code-sample">{CONNECTORS.executionCallbackExample}</pre>
                    </>
                  }
                >
                  <Input
                    value={executionCallbackUrl}
                    disabled={saving}
                    onChange={(e) => setExecutionCallbackUrl(e.target.value)}
                    placeholder="https://enterprise.example/baize/execute"
                  />
                </Field>
              </section>
              <section className="settings-advanced-block">
                <h4 className="settings-advanced-title">{TOOLS.captureSection}</h4>
                <CaptureSettingsFields
                  connectorId={id || 'new'}
                  connectorType={kind === 'plugin' ? 'http' : 'openapi'}
                  draft={captureDraft}
                  onDraftChange={(patch) => setCaptureDraft((d) => ({ ...d, ...patch }))}
                />
              </section>
            </details>
          )}
        </div>
      ) : (
        <div className="connector-permissions">
          <h3 className="connector-perms-title">{CONNECTORS.stepPermissions}</h3>
          <p className="connector-perms-intro">{isMcp ? CONNECTORS.permsIntroMcp : CONNECTORS.permsIntro}</p>
          {tools.length > 0 && (
            <Input
              className="connector-perms-search"
              value={toolQuery}
              disabled={saving}
              placeholder={CONNECTORS.permsSearch}
              aria-label={CONNECTORS.permsSearch}
              onChange={(e) => setToolQuery(e.target.value)}
            />
          )}
          {tools.length === 0 && <p className="settings-muted">{CONNECTORS.noToolsDiscovered}</p>}
          {tools.length > 0 && visibleTools.length === 0 && (
            <p className="settings-muted">{CONNECTORS.permsNoMatch}</p>
          )}
          {visibleTools.map((t) => {
            const label = t.title || t.name
            const showId = Boolean(t.title && t.title !== t.name)
            return (
              <div key={t.name} className="connector-perm-row">
                <div className="connector-perm-meta">
                  <span className="connector-perm-title">{label}</span>
                  {showId ? <span className="connector-perm-id">{t.name}</span> : null}
                  {t.description ? <span className="connector-perm-desc">{t.description}</span> : null}
                </div>
                <div className="connector-perm-flags">
                  {!isMcp && (
                    <label className="ui-checkbox-row">
                      <input
                        type="checkbox"
                        data-tool={t.name}
                        data-flag="login"
                        checked={selection[t.name]?.login ?? false}
                        disabled={saving}
                        onChange={() => setSelection((s) => toggleTool(s, t.name, 'login'))}
                      />
                      <span>{CONNECTORS.permLogin}</span>
                    </label>
                  )}
                  <label className="ui-checkbox-row">
                    <input
                      type="checkbox"
                      data-tool={t.name}
                      data-flag="approval"
                      checked={selection[t.name]?.approval ?? false}
                      disabled={saving}
                      onChange={() => setSelection((s) => toggleTool(s, t.name, 'approval'))}
                    />
                    <span>{CONNECTORS.permApproval}</span>
                  </label>
                </div>
              </div>
            )
          })}
        </div>
      )}
    </Modal>
  )
}
