import { useEffect, useState } from 'react'
import type { LoginEntry } from '../api'
import { fieldsFromEntry } from '../loginEntry'
import { LOGIN_AT } from '../strings'
import { Button, Field, Input, Modal } from './ui'

export interface LoginParamsModalProps {
  open: boolean
  entry: LoginEntry | null
  onSubmit: (args: Record<string, unknown>) => void | Promise<void>
  onCancel: () => void
}

export function LoginParamsModal({ open, entry, onSubmit, onCancel }: LoginParamsModalProps) {
  const fields = entry ? fieldsFromEntry(entry) : []
  const [values, setValues] = useState<Record<string, string>>({})
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open || !entry) {
      setValues({})
      setBusy(false)
      return
    }
    const next: Record<string, string> = {}
    for (const f of fieldsFromEntry(entry)) next[f.name] = ''
    setValues(next)
    setBusy(false)
  }, [open, entry])

  const canSubmit =
    !busy &&
    fields.length > 0 &&
    fields.every((f) => (values[f.name] ?? '').trim().length > 0)

  const submit = async () => {
    if (!entry || !canSubmit) return
    const args: Record<string, unknown> = {}
    for (const f of fields) args[f.name] = values[f.name] ?? ''
    setBusy(true)
    try {
      await onSubmit(args)
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open={open && entry != null}
      title={LOGIN_AT.paramsTitle}
      onClose={busy ? undefined : onCancel}
      footer={
        <>
          <Button variant="secondary" onClick={onCancel} disabled={busy}>
            {LOGIN_AT.paramsCancel}
          </Button>
          <Button variant="primary" onClick={submit} disabled={!canSubmit}>
            {LOGIN_AT.paramsSubmit}
          </Button>
        </>
      }
    >
      <div className="login-params-form">
        {fields.map((f) => (
          <Field key={f.name} label={f.name} required={f.required}>
            <Input
              name={f.name}
              type={f.type}
              autoComplete="off"
              value={values[f.name] ?? ''}
              onChange={(e) =>
                setValues((prev) => ({ ...prev, [f.name]: e.target.value }))
              }
              disabled={busy}
            />
          </Field>
        ))}
      </div>
    </Modal>
  )
}
