import { useEffect, useState } from 'react'
import { Button, Input, Modal } from '../../components/ui'
import { CHAT } from '../../strings'

export type RenameConversationDialogProps = {
  open: boolean
  initialTitle: string
  onClose: () => void
  onSave: (title: string) => Promise<void>
}

const MAX_TITLE_RUNES = 100

export function RenameConversationDialog({
  open,
  initialTitle,
  onClose,
  onSave,
}: RenameConversationDialogProps) {
  const [value, setValue] = useState(initialTitle)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setValue(initialTitle)
    setError(null)
  }, [open, initialTitle])

  const submit = async () => {
    const title = value.trim()
    if ([...title].length > MAX_TITLE_RUNES) {
      setError(CHAT.renameTooLong)
      return
    }
    setBusy(true)
    setError(null)
    try {
      await onSave(title)
      onClose()
    } catch {
      setError(CHAT.renameFailed)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={open}
      title={CHAT.renameDialogTitle}
      onClose={() => {
        if (!busy) onClose()
      }}
      footer={
        <>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            onClick={onClose}
            disabled={busy}
          >
            {CHAT.exportCancel}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="primary"
            onClick={() => void submit()}
            disabled={busy}
            data-testid="rename-conversation-confirm"
          >
            {busy ? CHAT.renameSaving : CHAT.renameSave}
          </Button>
        </>
      }
    >
      <label className="settings-muted">
        {CHAT.renameLabel}
        <Input
          value={value}
          maxLength={MAX_TITLE_RUNES}
          disabled={busy}
          onChange={(e) => setValue(e.target.value)}
          data-testid="rename-conversation-title"
        />
      </label>
      <p className="settings-muted">{CHAT.renameHint}</p>
      {error ? <p className="settings-error">{error}</p> : null}
    </Modal>
  )
}
