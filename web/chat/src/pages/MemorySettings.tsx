import { type FormEvent, useCallback, useEffect, useMemo, useState } from 'react'
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
import { groupMemory, type MemoryGroup } from '../memoryGroups'
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
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulkOpen, setBulkOpen] = useState(false)
  const [bulkError, setBulkError] = useState<string | null>(null)
  const [sourceFilter, setSourceFilter] = useState<'all' | 'explicit' | 'auto'>('all')
  const [search, setSearch] = useState('')

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

  useEffect(() => {
    setSelected(new Set())
    setBulkOpen(false)
    setBulkError(null)
  }, [sourceFilter, search])

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
      setSelected((prev) => {
        const next = new Set(prev)
        next.delete(pendingDelete.id)
        return next
      })
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

  const confirmBulkDelete = async () => {
    const ids = [...selected]
    if (ids.length === 0) return
    setBusy(true)
    setBulkError(null)
    const failed = new Set<string>()
    let firstError = ''
    for (const id of ids) {
      try {
        await deleteMemory(id)
      } catch (err) {
        failed.add(id)
        if (!firstError) {
          const f = friendlyError(err)
          firstError = f.detail ?? f.title
        }
      }
    }
    setSelected(failed)
    await refresh()
    setBusy(false)
    if (failed.size > 0) {
      setBulkError(firstError || MEMORY.loadFailed)
      push({ tone: 'error', title: MEMORY.loadFailed, detail: firstError })
      return
    }
    setBulkOpen(false)
    push({ tone: 'success', title: MEMORY.toastDeleted })
  }

  const toggleSelect = (id: string) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }

  const toggleGroup = (group: MemoryGroup, checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev)
      for (const entry of group.entries) {
        if (checked) next.add(entry.id)
        else next.delete(entry.id)
      }
      return next
    })
  }

  const filtered = useMemo(() => {
    return (items ?? []).filter((entry) => {
      if (sourceFilter !== 'all' && entry.source !== sourceFilter) return false
      const q = search.trim().toLowerCase()
      if (!q) return true
      return (
        (entry.key ?? '').toLowerCase().includes(q) || entry.text.toLowerCase().includes(q)
      )
    })
  }, [items, sourceFilter, search])
  const groups = useMemo(() => groupMemory(filtered), [filtered])

  const showEmpty = items !== null && items.length === 0
  const hasItems = items !== null && items.length > 0

  return (
    <div className="settings-panel settings-memory">
      <PageHeader title={MEMORY.title} description={MEMORY.description} />
      <ToastRegion toasts={toasts} onDismiss={dismiss} />

      {items === null && <p className="settings-muted">{MEMORY.loading}</p>}

      <form className="settings-form" onSubmit={(e) => void onCreate(e)}>
        <h2 className="settings-subheading">{MEMORY.add}</h2>
        <Field label={MEMORY.textLabel} hint={MEMORY.textHint}>
          <Textarea
            value={newText}
            onChange={(e) => setNewText(e.target.value)}
            disabled={busy}
            rows={3}
            placeholder={MEMORY.phText}
          />
        </Field>
        <Field label={MEMORY.keyLabel} hint={MEMORY.keyHint}>
          <Input
            value={newKey}
            onChange={(e) => setNewKey(e.target.value)}
            disabled={busy}
            placeholder={MEMORY.phKey}
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

      {hasItems && (
        <div className="settings-toolbar">
          <select
            name="memory-source"
            aria-label={MEMORY.filterLabel}
            value={sourceFilter}
            onChange={(e) => setSourceFilter(e.target.value as 'all' | 'explicit' | 'auto')}
            disabled={busy}
          >
            <option value="all">{MEMORY.filterAll}</option>
            <option value="explicit">{MEMORY.filterExplicit}</option>
            <option value="auto">{MEMORY.filterAuto}</option>
          </select>
          <Input
            name="memory-search"
            aria-label={MEMORY.searchPlaceholder}
            placeholder={MEMORY.searchPlaceholder}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            disabled={busy}
          />
          {selected.size > 0 ? (
            <span className="memory-selected">
              {MEMORY.selectedCount(selected.size)}
              <Button
                type="button"
                size="sm"
                variant="ghost"
                className="danger-text"
                disabled={busy}
                onClick={() => {
                  setBulkError(null)
                  setBulkOpen(true)
                }}
              >
                {MEMORY.bulkDelete}
              </Button>
            </span>
          ) : null}
        </div>
      )}

      {hasItems && filtered.length === 0 && (
        <p className="settings-muted">{MEMORY.noMatches}</p>
      )}

      <div className="memory-groups">
        {groups.map((group) => {
          const allSelected = group.entries.every((entry) => selected.has(entry.id))
          return (
            <section
              className="memory-group"
              key={group.key || '__ungrouped__'}
              data-testid="memory-group"
            >
              <div className="memory-group-head">
                <label className="settings-checkbox memory-group-select">
                  <input
                    type="checkbox"
                    checked={allSelected}
                    disabled={busy}
                    onChange={(e) => toggleGroup(group, e.target.checked)}
                    aria-label={MEMORY.selectAll}
                  />
                  <span className="memory-group-key">
                    {group.key || MEMORY.groupNoKey}
                  </span>
                </label>
                <span className="settings-muted">{MEMORY.groupCount(group.entries.length)}</span>
                {group.entries.length > 1 ? (
                  <Badge tone="warning">{MEMORY.conflictSameKey}</Badge>
                ) : null}
              </div>

              <ul className="settings-list">
                {group.entries.map((entry) => {
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
                        <div className="settings-tool-row">
                          <input
                            type="checkbox"
                            className="memory-entry-select"
                            checked={selected.has(entry.id)}
                            disabled={busy}
                            onChange={() => toggleSelect(entry.id)}
                            aria-label={MEMORY.selectEntry}
                          />
                          <div>
                            <p className="settings-tool-name">{entry.text}</p>
                            <p className="settings-muted">
                              {entry.key ? `${entry.key} · ` : ''}
                              <Badge>{sourceLabel(entry.source)}</Badge>
                              {group.nearDupIds.has(entry.id) ? (
                                <Badge tone="info">{MEMORY.nearDup}</Badge>
                              ) : null}
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
                      )}
                    </li>
                  )
                })}
              </ul>
            </section>
          )
        })}
      </div>

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

      <ConfirmDialog
        open={bulkOpen}
        danger
        title={MEMORY.bulkConfirmTitle}
        body={MEMORY.bulkConfirmBody(selected.size)}
        confirmText={MEMORY.bulkConfirmOk}
        busy={busy}
        error={bulkError}
        onCancel={() => {
          if (busy) return
          setBulkOpen(false)
          setBulkError(null)
        }}
        onConfirm={() => void confirmBulkDelete()}
      />
    </div>
  )
}
