import { Link } from 'react-router-dom'
import { Button, Field, Input, Modal, Select, Textarea } from '../../components/ui'
import { TOOLS } from '../../strings'
import { HTTP_METHODS } from './toolsSettingsHelpers'
import type { ToolsSettingsController } from './useToolsSettings'

type AddModalProps = Pick<
  ToolsSettingsController,
  | 'addModalOpen'
  | 'addFormError'
  | 'form'
  | 'setForm'
  | 'formConnectorId'
  | 'setFormConnectorId'
  | 'openConnectorIds'
  | 'submitting'
  | 'onAddSubmit'
  | 'closeAddModal'
>

export function ToolsAddModal({
  addModalOpen,
  addFormError,
  form,
  setForm,
  formConnectorId,
  setFormConnectorId,
  openConnectorIds,
  submitting,
  onAddSubmit,
  closeAddModal,
}: AddModalProps) {
  return (
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
            placeholder={TOOLS.phToolPath}
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
  )
}
