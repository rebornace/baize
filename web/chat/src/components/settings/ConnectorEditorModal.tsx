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
import type { PermissionTool } from '../../pages/connectorForms/permissions'
import type {
  ConnectorKind,
  FieldErrors,
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
  exportDbReadonly: false,
  oauthClientId: '', oauthClientSecret: '', oauthStatus: '',
}

export interface ConnectorEditorModalProps {
  kind: ConnectorKind
  open: boolean
  editing: boolean
  initial: ConnectorEditorInitial
  onClose: () => void
  formatError: (e: unknown) => string
  onSaveInfo: (conn: SavedConnection) => Promise<PermissionTool[]>
  // 连接信息真正保存成功时回调一次（失败不回调）；页面据此提示并刷新列表。
  onSavedInfo?: (conn: SavedConnection) => void
}

const EMPTY_INITIAL: ConnectorEditorInitial = {
  id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [],
}

export function ConnectorEditorModal(props: ConnectorEditorModalProps) {
  const { kind, open, editing, initial = EMPTY_INITIAL } = props
  // 父级可能渲染期内联传入新的 initial 对象；重置只允许在 open 变 true 时发生一次，
  // 故用 ref 读最新值、effect 仅依赖 open（I-3）。
  const initialRef = useRef(initial)
  initialRef.current = initial
  const [id, setId] = useState('')
  const [baseUrl, setBaseUrl] = useState('')
  const [importFormat, setImportFormat] = useState<ImportFormat>('auto')
  const [specContent, setSpecContent] = useState<string | null>(null)
  const [specFileName, setSpecFileName] = useState<string | null>(null)
  const [specUrl, setSpecUrl] = useState('')
  const [fieldErrors, setFieldErrors] = useState<FieldErrors>({})
  const [formError, setFormError] = useState<string | null>(null)
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
    setId(init.id)
    setBaseUrl(init.baseUrl)
    setImportFormat('auto')
    setSpecContent(null)
    setSpecFileName(null)
    setSpecUrl('')
    setFieldErrors({})
    setFormError(null)
    setSaving(false)
    setMcpForm(init.mcp
      ? connectorToMcpForm({ id: init.id, type: 'mcp', mcp: init.mcp })
      : { ...EMPTY_MCP_FORM })
    setMcpErrors({})
    setExecutionCallbackUrl(init.executionCallbackUrl ?? '')
    setCaptureDraft(captureToDraft(init.auth?.capture))
  }, [open])

  if (!open) return null

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
      await props.onSaveInfo(conn)
      props.onSavedInfo?.(conn)
      props.onClose()
    } catch (e) {
      setFormError(props.formatError(e))
      setSaving(false)
    }
  }

  const removeSpecFile = () => {
    setSpecContent(null)
    setSpecFileName(null)
    setFieldErrors((prev) => ({ ...prev, spec: undefined }))
  }

  return (
    <Modal
      open={open}
      title={title}
      onClose={saving ? undefined : props.onClose}
      footer={
        <>
          <Button variant="secondary" onClick={props.onClose} disabled={saving}>{CONNECTORS.cancel}</Button>
          <Button variant="primary" onClick={() => void saveInfo()} disabled={saving}>
            {saving ? CONNECTORS.saving : CONNECTORS.save}
          </Button>
        </>
      }
    >
      {formError && <p className="ui-inline-error" role="alert">{formError}</p>}
      <div className="connector-form">
        <Field label={CONNECTORS.fieldId} hint={CONNECTORS.fieldIdHint} required error={fieldErrors.id}>
          <Input value={id} disabled={editing || saving} placeholder={CONNECTORS.phId}
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
                  <Input value={mcpForm.command} disabled={saving} placeholder={CONNECTORS.phCommand}
                    onChange={(e) => {
                      setMcpForm((f) => ({ ...f, command: e.target.value }))
                      setMcpErrors((p) => ({ ...p, command: undefined }))
                    }} />
                </Field>
                <Field label={CONNECTORS.fieldArgs} hint={CONNECTORS.fieldArgsHint}>
                  <Textarea rows={3} value={mcpForm.argsText} disabled={saving} placeholder={CONNECTORS.phArgs}
                    onChange={(e) => setMcpForm((f) => ({ ...f, argsText: e.target.value }))} />
                </Field>
                <Field label={CONNECTORS.fieldEnv} hint={CONNECTORS.fieldEnvHint} error={mcpErrors.env}>
                  <Textarea rows={3} value={mcpForm.envText} disabled={saving} placeholder={CONNECTORS.phEnv}
                    onChange={(e) => {
                      setMcpForm((f) => ({ ...f, envText: e.target.value }))
                      setMcpErrors((p) => ({ ...p, env: undefined }))
                    }} />
                </Field>
              </>
            ) : (
              <>
                <Field label={CONNECTORS.fieldUrl} required error={mcpErrors.url}>
                  <Input value={mcpForm.url} disabled={saving} placeholder={CONNECTORS.phUrl}
                    onChange={(e) => {
                      setMcpForm((f) => ({ ...f, url: e.target.value }))
                      setMcpErrors((p) => ({ ...p, url: undefined }))
                    }} />
                </Field>
                <Field label={CONNECTORS.fieldHeaders} hint={CONNECTORS.fieldHeadersHint} error={mcpErrors.headers}>
                  <Textarea rows={3} value={mcpForm.headersText} disabled={saving} placeholder={CONNECTORS.phHeaders}
                    onChange={(e) => {
                      setMcpForm((f) => ({ ...f, headersText: e.target.value }))
                      setMcpErrors((p) => ({ ...p, headers: undefined }))
                    }} />
                </Field>
                <Field label={CONNECTORS.fieldOAuthClientId} hint={CONNECTORS.fieldOAuthClientIdHint}>
                  <Input
                    value={mcpForm.oauthClientId}
                    disabled={saving}
                    placeholder={CONNECTORS.phOAuthClientId}
                    autoComplete="off"
                    onChange={(e) => setMcpForm((f) => ({ ...f, oauthClientId: e.target.value }))}
                  />
                </Field>
                <Field label={CONNECTORS.fieldOAuthClientSecret} hint={CONNECTORS.fieldOAuthClientSecretHint}>
                  <Input
                    type="password"
                    value={mcpForm.oauthClientSecret}
                    disabled={saving}
                    placeholder={CONNECTORS.phOAuthSecret}
                    autoComplete="new-password"
                    onChange={(e) => setMcpForm((f) => ({ ...f, oauthClientSecret: e.target.value }))}
                  />
                </Field>
              </>
            )}
            <label className="ui-checkbox-row">
              <input
                type="checkbox"
                checked={mcpForm.exportDbReadonly}
                disabled={saving}
                data-testid="mcp-export-db-readonly"
                onChange={(e) => setMcpForm((f) => ({ ...f, exportDbReadonly: e.target.checked }))}
              />
              <span>
                {CONNECTORS.exportDbReadonly}
                <span className="ui-field-hint"> {CONNECTORS.exportDbReadonlyHint}</span>
              </span>
            </label>
          </>
        )}
        {!isMcp && (
          <Field label={CONNECTORS.fieldBaseUrl} required error={fieldErrors.baseUrl}>
            <Input value={baseUrl} disabled={saving}
              placeholder={isOpenapi ? CONNECTORS.phBaseUrlOpenapi : CONNECTORS.phBaseUrlPlugin}
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
                placeholder={CONNECTORS.phSpecUrl}
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
                  placeholder={CONNECTORS.phCallbackUrl}
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
    </Modal>
  )
}
