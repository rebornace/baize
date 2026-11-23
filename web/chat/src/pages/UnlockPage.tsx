import { type FormEvent, useState } from 'react'
import { getMe } from '../api'
import { clearControlToken, writeControlToken } from '../controlAuth'
import { UNLOCK } from '../strings'

export interface UnlockPageProps {
  onUnlocked: () => void
}

export function UnlockPage({ onUnlocked }: UnlockPageProps) {
  const [token, setToken] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)

  const onSubmit = async (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    setError(null)
    writeControlToken(token)
    try {
      const me = await getMe()
      if (me.role !== 'operator' && me.role !== 'admin') {
        clearControlToken()
        setError(UNLOCK.badToken)
        return
      }
      onUnlocked()
    } catch {
      clearControlToken()
      setError(UNLOCK.badToken)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="unlock-shell">
      <form className="unlock-card" onSubmit={(e) => void onSubmit(e)}>
        <h1 className="unlock-title">{UNLOCK.title}</h1>
        <input
          type="password"
          className="unlock-input"
          value={token}
          onChange={(e) => setToken(e.target.value)}
          autoComplete="current-password"
        />
        <button type="submit" className="btn primary" disabled={busy}>
          {UNLOCK.enter}
        </button>
        {error && <p className="unlock-error">{error}</p>}
      </form>
    </div>
  )
}
