import { Button } from './Button'
import { Modal } from './Modal'

export interface ConfirmDialogProps {
  open: boolean
  title: string
  body: string
  confirmText?: string
  cancelText?: string
  danger?: boolean
  busy?: boolean
  onConfirm: () => void
  onCancel: () => void
}

export function ConfirmDialog({
  open, title, body, confirmText = '确认', cancelText = '取消',
  danger, busy, onConfirm, onCancel,
}: ConfirmDialogProps) {
  return (
    <Modal
      open={open}
      title={title}
      onClose={onCancel}
      footer={
        <>
          <Button data-testid="confirm-cancel" variant="secondary" onClick={onCancel} disabled={busy}>
            {cancelText}
          </Button>
          <Button
            data-testid="confirm-ok"
            variant={danger ? 'danger' : 'primary'}
            onClick={onConfirm}
            disabled={busy}
          >
            {confirmText}
          </Button>
        </>
      }
    >
      <p className="confirm-body">{body}</p>
    </Modal>
  )
}
