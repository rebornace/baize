import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { Brain } from 'lucide-react'
import {
  createMemory,
  deleteMemory,
  listMemory,
  patchMemory,
  type MemoryEntry,
} from '../api'
import {
  Badge,
  Button,
  ConfirmDialog,
  EmptyState,
  Field,
  Input,
  PageHeader,
  Textarea,
  ToastRegion,
  useToast,
} from '../components/ui'
import { MEMORY, friendlyError } from '../strings'

function sourceLabel(source: string): string {
  switch (source) {
    case 'explicit':
      return MEMORY.sourceExplicit
    case 'auto':
      return MEMORY.sourceAuto
    default:
      return source
  }
}

export function MemorySettings() {
  const { toasts, push, dismiss } = useToast()
  const [items, setItems] = useState<MemoryEntry[] | null>(null)
  const [newText, setNewText] = useState('')
  const [newKey, setNewKey] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editText, setEditText] = useState('')
  const [editKey, setEditKey] = useState('')
  const [busy, setBusy] = useState(false)
  const [pendingDelete, setPendingDelete] = useState<MemoryEntry | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      setItems(await listMemory({ limit: 100 }))
    } catch (err) {
      setItems(null)
      const f = friendlyError(err)
      push({ tone: 'error', title: MEMORY.loadFailed, detail: f.detail ?? f.title })
    }
  }, [push])

  useEffect(() => {
    void refresh()
  }, [refresh])

  const onCreate = async (e: FormEvent) => {
    e.preventDefault()
    const text = newText.trim()
    if (!text) {
      push({ tone: 'error', title: MEMORY.toastNeedText })
      return
    }
    setBusy(true)
    try {
      const key = newKey.trim()
      await createMemory(key ? { text, key } : { text })
      setNewText('')
      setNewKey('')
      await refresh()
      push({ tone: 'success', title: MEMORY.toastCreated })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const beginEdit = (entry: MemoryEntry) => {
    setEditingId(entry.id)
    setEditText(entry.text)
    setEditKey(entry.key ?? '')
  }

  const cancelEdit = () => {
    setEditingId(null)
    setEditText('')
    setEditKey('')
  }

  const onSaveEdit = async (id: string) => {
    const text = editText.trim()
    if (!text) {
      push({ tone: 'error', title: MEMORY.toastNeedText })
      return
    }
    setBusy(true)
    try {
      await patchMemory(id, { text, key: editKey.trim() })
      cancelEdit()
      await refresh()
      push({ tone: 'success', title: MEMORY.toastUpdated })
    } catch (err) {
      const f = friendlyError(err)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const confirmDelete = async () => {
    if (!pendingDelete) return
    setBusy(true)
    setDeleteError(null)
    try {
      await deleteMemory(pendingDelete.id)
      push({ tone: 'success', title: MEMORY.toastDeleted })
      setPendingDelete(null)
      await refresh()
    } catch (err) {
      const f = friendlyError(err)
      const msg = f.detail ?? f.title
      setDeleteError(msg)
      push({ tone: 'error', title: f.title, detail: f.detail })
    } finally {
      setBusy(false)
    }
  }

  const showEmpty = items !== null && items.length === 0

  return (
    <div className="settings-panel settings-memory">
      <PageHeader title={MEMORY.title} description={MEMORY.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {items === null && <p className="settings-muted">加载中…</p>}

      <form className="settings-form" onSubmit={(e) => void onCreate(e)}>
        <h2 className="settings-subheading">{MEMORY.add}</h2>
        <Field label={MEMORY.textLabel} hint={MEMORY.textHint}>
          <Textarea
            value={newText}
            onChange={(e) => setNewText(e.target.value)}
            disabled={busy}
            rows={3}
            placeholder="例如：喜欢绿茶"
          />
        </Field>
        <Field label={MEMORY.keyLabel} hint={MEMORY.keyHint}>
          <Input
            value={newKey}
            onChange={(e) => setNewKey(e.target.value)}
            disabled={busy}
            placeholder="preference.tea"
          />
        </Field>
        <Button type="submit" variant="primary" disabled={busy}>
          {MEMORY.add}
        </Button>
      </form>

      {showEmpty && (
        <EmptyState
          icon={<Brain size={28} aria-hidden="true" />}
          title={MEMORY.emptyTitle}
          description={MEMORY.emptyDesc}
        />
      )}

      {items !== null && items.length > 0 && (
        <ul className="settings-list">
          {items.map((entry) => {
            const editing = editingId === entry.id
            return (
              <li key={entry.id} className="settings-list-item">
                {editing ? (
                  <div className="settings-form">
                    <Field label={MEMORY.textLabel}>
                      <Textarea
                        value={editText}
                        onChange={(e) => setEditText(e.target.value)}
                        disabled={busy}
                        rows={3}
                      />
                    </Field>
                    <Field label={MEMORY.keyLabel}>
                      <Input
                        value={editKey}
                        onChange={(e) => setEditKey(e.target.value)}
                        disabled={busy}
                      />
                    </Field>
                    <div className="settings-toolbar">
                      <Button
                        type="button"
                        variant="primary"
                        size="sm"
                        disabled={busy}
                        onClick={() => void onSaveEdit(entry.id)}
                      >
                        {MEMORY.save}
                      </Button>
                      <Button
                        type="button"
                        variant="ghost"
                        size="sm"
                        disabled={busy}
                        onClick={cancelEdit}
                      >
                        {MEMORY.cancel}
                      </Button>
                    </div>
                  </div>
                ) : (
                  <>
                    <div className="settings-tool-row">
                      <div>
                        <p className="settings-tool-name">{entry.text}</p>
                        <p className="settings-muted">
                          {entry.key ? `${entry.key} · ` : ''}
                          <Badge>{sourceLabel(entry.source)}</Badge>
                        </p>
                      </div>
                      <div className="accounts-actions">
                        <Button
                          type="button"
                          size="sm"
                          variant="secondary"
                          disabled={busy}
                          onClick={() => beginEdit(entry)}
                        >
                          {MEMORY.edit}
                        </Button>
                        <Button
                          type="button"
                          size="sm"
                          variant="ghost"
                          className="danger-text"
                          disabled={busy}
                          onClick={() => {
                            setDeleteError(null)
                            setPendingDelete(entry)
                          }}
                        >
                          {MEMORY.delete}
                        </Button>
                      </div>
                    </div>
                  </>
                )}
              </li>
            )
          })}
        </ul>
      )}

      <ConfirmDialog
        open={pendingDelete !== null}
        danger
        title={MEMORY.confirmDeleteTitle}
        body={MEMORY.confirmDeleteBody}
        confirmText={MEMORY.confirmDeleteOk}
        busy={busy}
        error={deleteError}
        onCancel={() => {
          if (busy) return
          setPendingDelete(null)
          setDeleteError(null)
        }}
        onConfirm={() => void confirmDelete()}
      />
    </div>
  )
}
