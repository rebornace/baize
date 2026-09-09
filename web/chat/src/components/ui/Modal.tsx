import { useEffect, useRef, type ReactNode } from 'react'

export interface ModalProps {
  open: boolean
  title: string
  onClose?: () => void
  children: ReactNode
  footer?: ReactNode
}

export function Modal({ open, title, onClose, children, footer }: ModalProps) {
  const panelRef = useRef<HTMLDivElement>(null)
  const previousActive = useRef<Element | null>(null)

  useEffect(() => {
    if (!open) return
    previousActive.current = document.activeElement
    const close = onClose
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') close?.()
    }
    document.addEventListener('keydown', onKey)
    const focusables = panelRef.current?.querySelectorAll<HTMLElement>(
      'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    )
    ;(focusables?.[0] ?? panelRef.current)?.focus()
    return () => {
      document.removeEventListener('keydown', onKey)
      ;(previousActive.current as HTMLElement | null)?.focus?.()
    }
  }, [open, onClose])

  if (!open) return null

  return (
    <div
      className="modal-overlay"
      data-testid="modal-overlay"
      onClick={(e) => { if (e.target === e.currentTarget) onClose?.() }}
    >
      <div
        ref={panelRef}
        className="modal-panel"
        data-testid="modal-panel"
        role="dialog"
        aria-modal="true"
        aria-label={title}
        tabIndex={-1}
      >
        <h2 className="modal-title">{title}</h2>
        <div className="modal-content">{children}</div>
        {footer && <div className="modal-footer">{footer}</div>}
      </div>
    </div>
  )
}
