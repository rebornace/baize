import { useCallback, useEffect, useState } from 'react'
import { File as FileIcon } from 'lucide-react'
import { Button, Modal } from '../../components/ui'
import { ChannelImage } from '../../components/ChannelImage'
import { fileKind } from '../../components/ChannelFileLink'
import {
  deleteConversationAttachments,
  listConversationAttachments,
  type ConversationAttachment,
} from '../../api/conversationAttachments'
import { CHAT } from '../../strings'

export interface AttachmentManagerProps {
  conversationId: string
  open: boolean
  onClose: () => void
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(1)} MB`
}

function formatTime(raw: string): string {
  const d = new Date(raw)
  return Number.isNaN(d.getTime()) ? raw : d.toLocaleString()
}

export function AttachmentManager({ conversationId, open, onClose }: AttachmentManagerProps) {
  const [items, setItems] = useState<ConversationAttachment[] | null>(null)
  const [selected, setSelected] = useState<Record<string, boolean>>({})
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      const body = await listConversationAttachments(conversationId)
      setItems(body.items)
      setSelected({})
      setError(null)
    } catch {
      setItems([])
      setError(CHAT.attachmentManagerLoadFailed)
    }
  }, [conversationId])

  useEffect(() => {
    if (open && conversationId) void load()
  }, [open, conversationId, load])

  const objects = Object.keys(selected).filter((k) => selected[k])
  const allSelected = items != null && items.length > 0 && objects.length === items.length

  const toggleAll = () => {
    if (!items) return
    if (allSelected) {
      setSelected({})
      return
    }
    const next: Record<string, boolean> = {}
    for (const it of items) next[it.object] = true
    setSelected(next)
  }

  const remove = async () => {
    if (objects.length === 0) return
    if (!window.confirm(CHAT.attachmentManagerConfirm(objects.length))) return
    setBusy(true)
    try {
      const res = await deleteConversationAttachments(conversationId, objects)
      await load()
      if (res.failed > 0) {
        setError(CHAT.attachmentManagerPartial(res.deleted, res.failed))
      }
    } catch {
      setError(CHAT.attachmentManagerFailed)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={open}
      title={CHAT.attachmentManagerTitle}
      onClose={onClose}
      footer={
        <>
          <Button type="button" size="sm" variant="secondary" onClick={onClose} disabled={busy}>
            {CHAT.attachmentManagerClose}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="danger"
            onClick={() => void remove()}
            disabled={busy || objects.length === 0}
            data-testid="attachment-manager-delete"
          >
            {CHAT.attachmentManagerDelete}
          </Button>
        </>
      }
    >
      <p className="settings-muted">{CHAT.attachmentManagerHint}</p>
      {error ? <p className="settings-error">{error}</p> : null}
      {items == null ? (
        <p className="settings-muted">{CHAT.attachmentManagerLoading}</p>
      ) : items.length === 0 ? (
        <p className="settings-muted">{CHAT.attachmentManagerEmpty}</p>
      ) : (
        <>
          <div className="attachment-manager-toolbar">
            <label className="settings-muted">
              <input
                type="checkbox"
                checked={allSelected}
                disabled={busy}
                onChange={toggleAll}
                data-testid="attachment-manager-select-all"
              />{' '}
              {CHAT.attachmentManagerSelectAll}
            </label>
            <span className="settings-muted">
              {CHAT.attachmentManagerSelected(objects.length)}
            </span>
          </div>
          <ul className="attachment-manager-list" data-testid="attachment-manager-list">
            {items.map((it) => {
              const isImage = it.media_type.startsWith('image/')
              const Kind = isImage ? null : fileKind(it.filename).icon
              return (
                <li key={it.object} className="attachment-manager-item">
                  <label>
                    <input
                      type="checkbox"
                      checked={!!selected[it.object]}
                      disabled={busy}
                      onChange={(e) =>
                        setSelected((prev) => ({ ...prev, [it.object]: e.target.checked }))
                      }
                    />
                  </label>
                  <div className="attachment-manager-thumb">
                    {isImage ? (
                      <ChannelImage
                        url={it.url}
                        name={it.filename}
                        deletable={false}
                        alt={it.filename}
                      />
                    ) : Kind ? (
                      <Kind size={22} />
                    ) : (
                      <FileIcon size={22} />
                    )}
                  </div>
                  <div className="attachment-manager-meta">
                    <span className="attachment-manager-name">{it.filename}</span>
                    <span className="settings-muted">
                      {formatBytes(it.size_bytes)} · {formatTime(it.created_at)}
                    </span>
                  </div>
                </li>
              )
            })}
          </ul>
        </>
      )}
    </Modal>
  )
}
