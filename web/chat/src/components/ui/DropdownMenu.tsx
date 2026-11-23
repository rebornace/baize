import { useCallback, useEffect, useId, useRef, useState, type ReactNode } from 'react'

export interface MenuItem {
  id: string
  label: string
  icon?: ReactNode
  destructive?: boolean
  onSelect: () => void
}

export interface DropdownMenuProps {
  triggerLabel: string
  items: MenuItem[]
  triggerClassName?: string
  disabled?: boolean
  children?: ReactNode
}

export function DropdownMenu({ triggerLabel, items, triggerClassName, disabled = false, children }: DropdownMenuProps) {
  const [open, setOpen] = useState(false)
  const [active, setActive] = useState(0)
  const rootRef = useRef<HTMLDivElement>(null)
  const menuId = useId()

  const close = useCallback(() => setOpen(false), [])

  useEffect(() => {
    if (!open) return
    const onDown = (e: MouseEvent) => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) close()
    }
    const onKey = (e: KeyboardEvent) => { if (e.key === 'Escape') close() }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [open, close])

  const choose = (item: MenuItem) => { item.onSelect(); close() }

  const onMenuKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown') { e.preventDefault(); setActive((a) => (a + 1) % items.length) }
    else if (e.key === 'ArrowUp') { e.preventDefault(); setActive((a) => (a - 1 + items.length) % items.length) }
    else if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); const it = items[active]; if (it) choose(it) }
  }

  const onTriggerKey = (e: React.KeyboardEvent) => {
    if (e.key === 'ArrowDown' && open) {
      e.preventDefault()
      setActive(0)
    }
  }

  return (
    <div className="dropdown" ref={rootRef}>
      <button
        type="button"
        className={`dropdown-trigger ${triggerClassName ?? ''}`.trim()}
        data-testid="dropdown-trigger"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-controls={open ? menuId : undefined}
        aria-label={triggerLabel}
        title={triggerLabel}
        disabled={disabled}
        onClick={() => { if (!disabled) { setOpen((v) => !v); setActive(0) } }}
        onKeyDown={onTriggerKey}
      >
        {children ?? '⋯'}
      </button>
      {open && (
        <div
          id={menuId}
          className="dropdown-menu"
          role="menu"
          aria-label={triggerLabel}
          onKeyDown={onMenuKey}
        >
          {items.map((item, i) => (
            <button
              key={item.id}
              type="button"
              role="menuitem"
              className={`dropdown-item${item.destructive ? ' danger' : ''}${i === active ? ' active' : ''}`}
              tabIndex={-1}
              onMouseEnter={() => setActive(i)}
              onClick={() => choose(item)}
              ref={(el) => { if (i === active && open) el?.focus() }}
            >
              {item.icon}
              <span>{item.label}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
