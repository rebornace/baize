import { useState } from 'react'
import { getRunBinding } from '../api'
import { ApiError } from '../api/http'
import { Button, Modal } from './ui'
import { CHAT, friendlyError } from '../strings'

export type RunBindingButtonProps = {
  runId: string
}

export function RunBindingButton({ runId }: RunBindingButtonProps) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [jsonText, setJsonText] = useState('')
  const [hidden, setHidden] = useState(false)

  const openBinding = async () => {
    setBusy(true)
    setError(null)
    try {
      const body = await getRunBinding(runId)
      setJsonText(JSON.stringify(body.binding ?? body, null, 2))
      setOpen(true)
    } catch (e) {
      if (e instanceof ApiError && (e.status === 404 || e.code === 'binding_not_found')) {
        setHidden(true)
        return
      }
      const f = friendlyError(e)
      setError(f.detail ? `${f.title}: ${f.detail}` : f.title)
      setOpen(true)
    } finally {
      setBusy(false)
    }
  }

  if (hidden || !runId.trim()) return null

  return (
    <>
      <button
        type="button"
        className="btn ghost sm"
        data-testid="run-binding-open"
        disabled={busy}
        onClick={() => void openBinding()}
      >
        {busy ? CHAT.bindingLoading : CHAT.viewBinding}
      </button>
      <Modal
        open={open}
        title={CHAT.bindingTitle}
        onClose={() => setOpen(false)}
        footer={
          <Button variant="primary" onClick={() => setOpen(false)}>
            {CHAT.bindingClose}
          </Button>
        }
      >
        <p className="settings-muted">{CHAT.bindingHint}</p>
        {error ? (
          <p className="ui-inline-error" role="alert">
            {error}
          </p>
        ) : (
          <pre className="settings-code-sample" data-testid="run-binding-body" style={{ maxHeight: '50vh', overflow: 'auto' }}>
            {jsonText}
          </pre>
        )}
      </Modal>
    </>
  )
}
