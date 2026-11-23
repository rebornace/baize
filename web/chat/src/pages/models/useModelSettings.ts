import { useCallback, useEffect, useState } from 'react'
import {
  deleteModelProfile,
  listModelProfiles,
  updateModelProfile,
  type ModelProfile,
} from '../../api'
import { useToast } from '../../components/ui'
import { useGate } from '../../gateContext'
import { MODELS, modelErrorText } from '../../strings'
import {
  EMPTY_PROFILE_FORM,
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
  const [addOpen, setAddOpen] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<ProfileFormState>(EMPTY_PROFILE_FORM)
  const [formError, setFormError] = useState<string | null>(null)
  const [pendingDelete, setPendingDelete] = useState<ModelProfile | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const editOpen = editingId != null
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
    setAddOpen(true)
  }

  const closeCreate = () => {
    setAddOpen(false)
  }

  const startEdit = (p: ModelProfile) => {
    setEditingId(p.id)
    setForm(profileToForm(p))
    setFormError(null)
  }

  // Save handles the edit form only; new models go through AddModelModal.
  const save = async () => {
    setFormError(null)
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
  const editTitle = `${MODELS.edit} ${profiles.find((p) => p.id === editingId)?.name ?? ''}`.trim()

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
    addOpen,
    editOpen,
    editingId,
    isLastModel,
    showEmpty,
    editTitle,
    closeEditor,
    closeCreate,
    openCreate,
    startEdit,
    save,
    beginDelete,
    cancelDelete,
    confirmDelete,
    refresh: load,
  }
}
