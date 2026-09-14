import { useEffect, useState } from 'react'
import type { LoginEntry } from '../api'
import { LOGIN_AT } from '../strings'
import { Button, Modal } from './ui'
import { LoginParamsModal } from './LoginParamsModal'

export interface LoginPickerProps {
  open: boolean
  entries: LoginEntry[]
  onClose: () => void
  onPick: (entry: LoginEntry, args?: Record<string, unknown>) => void | Promise<void>
  title?: string
  /** Shown when entries is empty (e.g. no connector vs no matches). */
  emptyMessage?: string
}

export function LoginPicker({
  open,
  entries,
  onClose,
  onPick,
  title = LOGIN_AT.sectionLogin,
  emptyMessage = LOGIN_AT.pickerEmpty,
}: LoginPickerProps) {
  const [paramsEntry, setParamsEntry] = useState<LoginEntry | null>(null)
  const [busy, setBusy] = useState(false)

  useEffect(() => {
    if (!open) {
      setParamsEntry(null)
      setBusy(false)
    }
  }, [open])

  const pick = async (entry: LoginEntry, args?: Record<string, unknown>) => {
    setBusy(true)
    try {
      await onPick(entry, args)
      setParamsEntry(null)
      onClose()
    } finally {
      setBusy(false)
    }
  }

  const choose = (entry: LoginEntry) => {
    if ((entry.required ?? []).length === 0) {
      void pick(entry)
      return
    }
    setParamsEntry(entry)
  }

  return (
    <>
      <Modal
        open={open && paramsEntry == null}
        title={title}
        onClose={busy ? undefined : onClose}
        footer={
          <Button variant="secondary" onClick={onClose} disabled={busy}>
            {LOGIN_AT.paramsCancel}
          </Button>
        }
      >
        {entries.length === 0 ? (
          <p className="login-picker-empty">{emptyMessage}</p>
        ) : (
          <ul className="login-picker-list" role="listbox" aria-label={title}>
            {entries.map((entry) => (
              <li key={entry.id}>
                <button
                  type="button"
                  className="login-picker-item"
                  disabled={busy}
                  onClick={() => choose(entry)}
                >
                  <span className="login-picker-title">
                    {entry.title}
                    {entry.logged_in ? (
                      <span className="composer-complete-badge">{LOGIN_AT.loggedInBadge}</span>
                    ) : null}
                  </span>
                  <span className="login-picker-meta">
                    {entry.connector_title || entry.connector_id}
                    {entry.tool_name ? ` · ${entry.tool_name}` : ''}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        )}
      </Modal>
      <LoginParamsModal
        open={open && paramsEntry != null}
        entry={paramsEntry}
        onCancel={() => setParamsEntry(null)}
        onSubmit={async (args) => {
          if (!paramsEntry) return
          await pick(paramsEntry, args)
        }}
      />
    </>
  )
}
