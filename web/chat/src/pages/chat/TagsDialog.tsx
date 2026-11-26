import { useEffect, useState } from 'react'
import { Button, Input, Modal } from '../../components/ui'
import { CHAT } from '../../strings'

export type TagsDialogProps = {
  open: boolean
  initialTags: string[]
  onClose: () => void
  onSave: (tags: string[]) => Promise<void>
}

export function TagsDialog({ open, initialTags, onClose, onSave }: TagsDialogProps) {
  const [value, setValue] = useState(initialTags.join(', '))
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setValue(initialTags.join(', '))
    setError(null)
  }, [open, initialTags])

  const submit = async () => {
    const tags = value
      .split(/[,，]/)
      .map((tag) => tag.trim())
      .filter(Boolean)
    if (tags.some((tag) => [...tag].length > 24)) {
      setError(CHAT.tagsTooLong)
      return
    }
    if (tags.length > 8) {
      setError(CHAT.tagsTooMany)
      return
    }
    setBusy(true)
    setError(null)
    try {
      await onSave(tags)
      onClose()
    } catch {
      setError(CHAT.tagsFailed)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={open}
      title={CHAT.tagsDialogTitle}
      onClose={() => {
        if (!busy) onClose()
      }}
      footer={
        <>
          <Button type="button" size="sm" variant="secondary" onClick={onClose} disabled={busy}>
            {CHAT.exportCancel}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="primary"
            onClick={() => void submit()}
            disabled={busy}
            data-testid="tags-conversation-confirm"
          >
            {busy ? CHAT.renameSaving : CHAT.tagsSave}
          </Button>
        </>
      }
    >
      <label className="settings-muted">
        {CHAT.tagsLabel}
        <Input
          value={value}
          disabled={busy}
          onChange={(e) => setValue(e.target.value)}
          data-testid="tags-conversation-input"
          placeholder="work, urgent"
        />
      </label>
      <p className="settings-muted">{CHAT.tagsHint}</p>
      {error ? <p className="settings-error">{error}</p> : null}
    </Modal>
  )
}
