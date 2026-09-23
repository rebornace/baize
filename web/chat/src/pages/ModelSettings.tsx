import { Cpu } from 'lucide-react'
import {
  Button,
  ConfirmDialog,
  EmptyState,
  Modal,
  PageHeader,
  ToastRegion,
} from '../components/ui'
import { MODELS } from '../strings'
import { AddModelModal } from './models/AddModelModal'
import { ModelProfileList } from './models/ModelProfileList'
import { ProfileFields } from './models/ProfileFields'
import {
  EMPTY_PROFILE_FORM,
  buildCreatePayload,
  buildPatchPayload,
  profileToForm,
  type ModelProfilePayload,
  type ProfileFormState,
} from './models/modelSettingsHelpers'
import { useModelSettings } from './models/useModelSettings'

export {
  EMPTY_PROFILE_FORM,
  buildCreatePayload,
  buildPatchPayload,
  profileToForm,
  ModelProfileList,
}
export type { ModelProfilePayload, ProfileFormState }

export function ModelSettings() {
  const s = useModelSettings()

  const headerAdd =
    s.showEmpty || s.readOnly ? undefined : (
      <Button variant="primary" size="sm" onClick={s.openCreate}>
        {MODELS.add}
      </Button>
    )

  return (
    <div className="settings-panel settings-models">
      <PageHeader title={MODELS.title} description={MODELS.description} actions={headerAdd} />
      <ToastRegion toasts={s.toasts} onDismiss={s.dismiss} />

      {s.loading && <p className="settings-muted">加载中…</p>}

      {s.showEmpty && (
        <EmptyState
          icon={<Cpu size={28} aria-hidden="true" />}
          title={MODELS.emptyTitle}
          description={s.readOnly ? MODELS.emptyDescOperator : MODELS.emptyDescAdmin}
          action={
            s.readOnly ? undefined : (
              <Button variant="primary" onClick={s.openCreate}>
                {MODELS.add}
              </Button>
            )
          }
        />
      )}

      {!s.loading && s.profiles.length > 0 && (
        <ModelProfileList
          profiles={s.profiles}
          busy={s.busy || s.editOpen || s.addOpen}
          readOnly={s.readOnly}
          onEdit={s.startEdit}
          onDelete={s.beginDelete}
        />
      )}

      {!s.readOnly && (
        <AddModelModal
          open={s.addOpen}
          onClose={s.closeCreate}
          onImported={s.refresh}
        />
      )}

      {!s.readOnly && (
        <Modal
          open={s.editOpen}
          title={s.editTitle}
          onClose={s.busy ? undefined : s.closeEditor}
          footer={
            <>
              <Button variant="ghost" disabled={s.busy} onClick={s.closeEditor}>
                {MODELS.cancel}
              </Button>
              <Button disabled={s.busy} onClick={() => void s.save()}>
                {MODELS.save}
              </Button>
            </>
          }
        >
          <ProfileFields
            form={s.form}
            setForm={s.setForm}
            busy={s.busy}
          />
          {s.formError && (
            <p className="ui-inline-error" role="alert">
              {s.formError}
            </p>
          )}
        </Modal>
      )}

      {!s.readOnly && (
        <ConfirmDialog
          open={!!s.pendingDelete}
          danger
          title={MODELS.confirmDeleteTitle}
          body={
            s.isLastModel
              ? `${MODELS.confirmDeleteBody}\n${MODELS.confirmDeleteLast}`
              : MODELS.confirmDeleteBody
          }
          confirmText={MODELS.confirmDeleteOk}
          busy={s.busy}
          error={s.deleteError}
          onCancel={s.cancelDelete}
          onConfirm={() => void s.confirmDelete()}
        />
      )}
    </div>
  )
}
