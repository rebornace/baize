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
      if (e.key === 'Escape') {
        close?.()
        return
      }
      if (e.key !== 'Tab') return
      const nodes = Array.from(
        panelRef.current?.querySelectorAll<HTMLElement>(
          'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
        ) ?? [],
      ).filter((el) => !el.hasAttribute('disabled'))
      // 无可用控件时焦点留在 panel（panel 带 tabIndex={-1}）。
      e.preventDefault()
      if (nodes.length === 0) {
        panelRef.current?.focus()
        return
      }
      // 显式接管 Tab：jsdom 不会原生移动焦点，且这样能保证 disabled
      // 被跳过、首尾环绕；当前焦点不在列表内（背景/panel）时分别落到首/尾。
      const current = nodes.indexOf(document.activeElement as HTMLElement)
      if (e.shiftKey) {
        const prev = current <= 0 ? nodes[nodes.length - 1] : nodes[current - 1]
        prev.focus()
      } else {
        const next = current === -1 || current === nodes.length - 1 ? nodes[0] : nodes[current + 1]
        next.focus()
      }
    }
    document.addEventListener('keydown', onKey)
    const initialFocusables = Array.from(
      panelRef.current?.querySelectorAll<HTMLElement>(
        'button, [href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
      ) ?? [],
    ).filter((el) => !el.hasAttribute('disabled'))
    ;(initialFocusables[0] ?? panelRef.current)?.focus()
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
