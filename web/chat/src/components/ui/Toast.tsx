import { useCallback, useRef, useState } from 'react'

export type ToastTone = 'success' | 'error' | 'info'

export interface ToastInput {
  tone: ToastTone
  title: string
  detail?: string
}

export interface ToastItem extends ToastInput {
  id: number
}

const DURATION_MS: Record<ToastTone, number> = {
  success: 4000,
  info: 4000,
  error: 6000,
}
const MAX_TOASTS = 3

export interface ToastApi {
  toasts: ToastItem[]
  push: (t: ToastInput) => void
  dismiss: (id: number) => void
}

export function useToast(): ToastApi {
  const [toasts, setToasts] = useState<ToastItem[]>([])
  const nextId = useRef(1)

  const dismiss = useCallback((id: number) => {
    setToasts((prev) => prev.filter((t) => t.id !== id))
  }, [])

  const push = useCallback(
    (t: ToastInput) => {
      const id = nextId.current++
      setToasts((prev) => [...prev, { ...t, id }].slice(-MAX_TOASTS))
      window.setTimeout(() => dismiss(id), DURATION_MS[t.tone])
    },
    [dismiss],
  )

  return { toasts, push, dismiss }
}

export function ToastRegion({
  toasts,
  onDismiss,
}: {
  toasts: ToastItem[]
  onDismiss: (id: number) => void
}) {
  return (
    <div className="toast-region" aria-live="polite" aria-atomic="false">
      {toasts.map((t) => (
        <div
          key={t.id}
          data-testid="toast"
          className={`toast toast-${t.tone}`}
          role="status"
        >
          <div className="toast-body">
            <p className="toast-title">{t.title}</p>
            {t.detail && <p className="toast-detail">{t.detail}</p>}
          </div>
          <button
            type="button"
            className="toast-close"
            data-testid="toast-close"
            aria-label="关闭提示"
            onClick={() => onDismiss(t.id)}
          >
            ×
          </button>
        </div>
      ))}
    </div>
  )
}
