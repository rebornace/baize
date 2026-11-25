import { useEffect, useState } from 'react'
import type { Workspace } from '../../api'
import { Button, Modal, Select } from '../../components/ui'
import { CHAT } from '../../strings'

export type MoveConversationDialogProps = {
  open: boolean
  targets: Workspace[]
  onClose: () => void
  onMove: (workspaceId: string) => Promise<void>
}

/** Moves the open conversation to another workspace. */
export function MoveConversationDialog({
  open,
  targets,
  onClose,
  onMove,
}: MoveConversationDialogProps) {
  const [target, setTarget] = useState('')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    if (!open) return
    setTarget(targets[0]?.id ?? '')
    setError(null)
  }, [open, targets])

  const submit = async () => {
    if (!target) return
    setBusy(true)
    setError(null)
    try {
      await onMove(target)
      onClose()
    } catch {
      setError(CHAT.moveConversationFailed)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={open}
      title={CHAT.moveConversationTitle}
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
            disabled={busy || !target}
            data-testid="move-conversation-confirm"
          >
            {busy ? CHAT.moveConversationBusy : CHAT.moveConversationConfirm}
          </Button>
        </>
      }
    >
      <p className="settings-muted">{CHAT.moveConversationHint}</p>
      <label className="settings-muted">
        {CHAT.moveConversationTarget}
        <Select
          value={target}
          disabled={busy}
          onChange={(e) => setTarget(e.target.value)}
          data-testid="move-conversation-target"
        >
          {targets.map((w) => (
            <option key={w.id} value={w.id}>
              {w.name}
            </option>
          ))}
        </Select>
      </label>
      {error ? <p className="settings-error">{error}</p> : null}
    </Modal>
  )
}
