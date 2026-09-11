import { type ChangeEvent, useEffect, useRef, useState } from 'react'
import { Button, Field, Input, Modal, Select } from '../ui'
import { CONNECTORS } from '../../strings'
import { validateConnection } from '../../pages/connectorForms/validate'
import {
  emptySelection,
  selectionFromLists,
  toNameLists,
  toggleTool,
} from '../../pages/connectorForms/permissions'
import type { ConnectorKind, FieldErrors, PermissionSelection } from '../../pages/connectorForms/types'
import type { ImportFormat } from '../../api'

export interface ConnectorEditorInitial {
  id: string
  baseUrl: string
  tools: { name: string }[]
  loginNames: string[]
  approvalNames: string[]
}

export interface ConnectorEditorModalProps {
  kind: ConnectorKind
  open: boolean
  editing: boolean
  initial: ConnectorEditorInitial
  onClose: () => void
  formatError: (e: unknown) => string
  onSaveInfo: (input: {
    id: string
    baseUrl: string
    spec?: { content?: string; url?: string }
    importFormat: ImportFormat
  }) => Promise<{ name: string }[]>
  onSavePermissions: (
    id: string,
    baseUrl: string,
    loginNames: string[],
    approvalNames: string[],
  ) => Promise<void>
  // 第一步连接信息真正保存成功时回调一次（失败不回调）；页面据此立即提示并
  // 刷新列表，避免用户新建后直接跳过/关闭弹窗看不到新连接器。
  onSavedInfo?: (id: string) => void
}

const EMPTY_INITIAL: ConnectorEditorInitial = {
  id: '', baseUrl: '', tools: [], loginNames: [], approvalNames: [],
}

interface NamedTool {
  name: string
}

/**
 * 重建第 2 步的权限选择：保留 prev 中同名工具已有的勾选，
 * 仅对新出现的工具使用 fallback（编辑回显 / 新建全不勾）。
 */
const reselect = (tools: NamedTool[], fallback: PermissionSelection) =>
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
  const [tools, setTools] = useState<{ name: string }[]>([])
  const [selection, setSelection] = useState<PermissionSelection>({})
  const [saving, setSaving] = useState(false)

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
    setTools(init.tools)
    setSelection(selectionFromLists(init.tools, init.loginNames, init.approvalNames))
  }, [open])

  if (!open) return null

  const isOpenapi = kind === 'openapi'
  const title = editing
    ? (isOpenapi ? CONNECTORS.editOpenapi : CONNECTORS.editPlugin)
    : (isOpenapi ? CONNECTORS.addOpenapi : CONNECTORS.addPlugin)

  const onSpecFile = (e: ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file) return
    const reader = new FileReader()
    reader.onload = () => {
      setSpecContent(typeof reader.result === 'string' ? reader.result : '')
      setSpecFileName(file.name)
      setSpecUrl('')
      setFieldErrors((prev) => ({ ...prev, spec: undefined }))
    }
    reader.onerror = () => {
      setFormError(CONNECTORS.specReadFailed)
    }
    reader.readAsText(file)
  }

  const saveInfo = async () => {
    const hasSpec = specContent != null || specUrl.trim() !== ''
    const result = validateConnection({ kind, id, baseUrl, hasSpec, editing })
    if (!result.ok) {
      setFieldErrors(result.fieldErrors)
      return
    }
    setFieldErrors({})
    setFormError(null)
    setSaving(true)
    try {
      const discovered = await props.onSaveInfo({
        id: result.id,
        baseUrl: result.baseUrl,
        spec: isOpenapi && hasSpec
          ? { content: specContent ?? undefined, url: specContent == null ? specUrl.trim() : undefined }
          : undefined,
        importFormat,
      })
      const nextTools = discovered.length > 0 ? discovered : initial.tools
      const fallback = editing
        ? selectionFromLists(nextTools, initial.loginNames, initial.approvalNames)
        : emptySelection(nextTools)
      setTools(nextTools)
      // 保留本次打开期间已勾选过的同名工具权限（I-1）
      setSelection(reselect(nextTools, fallback))
      setStep(2)
      // 第一步已真正保存（连接器已创建/更新）：通知页面提示并刷新列表，
      // 这样用户随后直接「暂不设置」或关闭弹窗也能看到新连接器。
      props.onSavedInfo?.(result.id)
    } catch (e) {
      setFormError(props.formatError(e))
    } finally {
      setSaving(false)
    }
  }

  const gotoPermissions = () => {
    const fallback = selectionFromLists(initial.tools, initial.loginNames, initial.approvalNames)
    setTools(initial.tools)
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
      await props.onSavePermissions(id.trim(), baseUrl.trim(), loginNames, approvalNames)
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
          <Field label={CONNECTORS.fieldBaseUrl} required error={fieldErrors.baseUrl}>
            <Input value={baseUrl} disabled={saving}
              placeholder={isOpenapi ? 'https://api.example.com' : 'http://127.0.0.1:19090'}
              onChange={(e) => { setBaseUrl(e.target.value); setFieldErrors((p) => ({ ...p, baseUrl: undefined })) }} />
          </Field>
          {isOpenapi && (
            <>
              <Field label={CONNECTORS.fieldSpec} hint={CONNECTORS.fieldSpecHint} error={fieldErrors.spec}>
                <Input type="file" accept=".json,.yaml,.yml" disabled={saving} onChange={onSpecFile} />
              </Field>
              {specFileName && (
                <p className="ui-field-hint connector-spec-file-row">
                  <span>{CONNECTORS.specFileChosen(specFileName)}</span>
                  <Button type="button" variant="ghost" size="sm" disabled={saving} onClick={removeSpecFile}>
                    {CONNECTORS.specRemoveFile}
                  </Button>
                </p>
              )}
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
        </div>
      ) : (
        <div className="connector-permissions">
          <h3 className="connector-perms-title">{CONNECTORS.stepPermissions}</h3>
          <p className="connector-perms-intro">{CONNECTORS.permsIntro}</p>
          {tools.length === 0 && <p className="settings-muted">暂无已识别工具。</p>}
          {tools.map((t) => (
            <div key={t.name} className="connector-perm-row">
              <span className="connector-perm-name">{t.name}</span>
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
          ))}
        </div>
      )}
    </Modal>
  )
}
