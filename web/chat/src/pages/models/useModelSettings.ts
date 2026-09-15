import { useCallback, useEffect, useState } from 'react'
import {
  createModelProfile,
  deleteModelProfile,
  listModelProfiles,
  updateModelProfile,
  type ModelProfile,
} from '../../api'
import { useToast } from '../../components/ui'
import { useGate } from '../../gateContext'
import { MODELS, modelErrorText } from '../../strings'
import {
  CREATE_ID,
  EMPTY_PROFILE_FORM,
  buildCreatePayload,
  buildPatchPayload,
  profileToForm,
  type ProfileFormState,
} from './modelSettingsHelpers'

export function useModelSettings() {
  const { role } = useGate()
  const readOnly = role !== 'admin'
  const { toasts, push, dismiss } = useToast()
  const [profiles, setProfiles] = useState<ModelProfile[]>([])
  const [loading, setLoading] = useState(true)
  const [loadFailed, setLoadFailed] = useState(false)
  const [busy, setBusy] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM)
  const [formError, setFormError] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] = useState<ModelProfile | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const modalOpen = editingId != null
  const isCreate = editingId === CREATE_ID
  const isLastModel = profiles.length === 1

  const load = useCallback(async () => {
    setLoading(true)
    try {
      const list = await listModelProfiles()
      setProfiles(list)
      setLoadFailed(false)
    } catch (err) {
      setLoadFailed(true)
      const f = modelErrorText(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setLoading(false)
    }
  }, [push])

  useEffect(() => {
    void load()
  }, [load])

  const closeEditor = () => {
    if (busy) return
    setEditingId(null)
    setForm(EMPTY_PROFILE_FORM)
    setFormError(null)
  }

  const openCreate = () => {
    setEditingId(CREATE_ID)
    setForm(EMPTY_PROFILE_FORM)
    setFormError(null)
  }

  const startEdit = (p: ModelProfile) => {
    setEditingId(p.id)
    setForm(profileToForm(p))
    setFormError(null)
  }

  const save = async () => {
    setFormError(null)
    if (isCreate) {
      const built = buildCreatePayload(form)
      if (!built.ok) {
        setFormError(built.message)
        return
      }
      setBusy(true)
      try {
        await createModelProfile(built.payload)
        setEditingId(null)
        setForm(EMPTY_PROFILE_FORM)
        setFormError(null)
        push({ tone: 'success', title: MODELS.toastSaved })
        await load()
      } catch (err) {
        const f = modelErrorText(err)
        setFormError(f.detail ? `${f.title} ${f.detail}` : f.title)
        push({ tone: 'error', title: f.title, detail: f.detail })
      } finally {
        setBusy(false)
      }
      return
    }
    if (!editingId) return
    const original = profiles.find((p) => p.id === editingId)
    if (!original) return
    const name = form.name.trim()
    if (!name) {
      setFormError(MODELS.errNameRequired)
      return
    }
    const payload = buildPatchPayload(form, original)
    setBusy(true)
    try {
      await updateModelProfile(editingId, payload)
      setEditingId(null)
      setForm(EMPTY_PROFILE_FORM)
      setFormError(null)
      push({ tone: 'success', title: MODELS.toastSaved })
      await load()
    } catch (err) {
      const f = modelErrorText(err)
      setFormError(f.detail ? `${f.title} ${f.detail}` : f.title)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const beginDelete = (p: ModelProfile) => {
    setDeleteError(null)
    setPendingDelete(p)
  }

  const cancelDelete = () => {
    if (busy) return
    setPendingDelete(null)
    setDeleteError(null)
  }

  const confirmDelete = async () => {
    if (!pendingDelete) return
    setBusy(true)
    setDeleteError(null)
    try {
      await deleteModelProfile(pendingDelete.id)
      if (editingId === pendingDelete.id) {
        setEditingId(null)
        setForm(EMPTY_PROFILE_FORM)
        setFormError(null)
      }
      push({ tone: 'success', title: MODELS.toastDeleted, detail: pendingDelete.name })
      setPendingDelete(null)
      await load()
    } catch (err) {
      const f = modelErrorText(err)
      setDeleteError(f.detail ? `${f.title} ${f.detail}` : f.title)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const showEmpty = !loading && profiles.length === 0 && !loadFailed
  const modalTitle = isCreate
    ? MODELS.add
    : `${MODELS.edit} ${profiles.find((p) => p.id === editingId)?.name ?? ''}`.trim()

  return {
    readOnly,
    toasts,
    dismiss,
    profiles,
    loading,
    busy,
    form,
    setForm,
    formError,
    pendingDelete,
    deleteError,
    modalOpen,
    isCreate,
    isLastModel,
    showEmpty,
    modalTitle,
    closeEditor,
    openCreate,
    startEdit,
    save,
    beginDelete,
    cancelDelete,
    confirmDelete,
  }
}
