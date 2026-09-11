import { useCallback, useEffect, useRef } from 'react'
import { applySidebarWidth, DEFAULT_SIDEBAR_WIDTH, persistSidebarWidth } from '../sidebarResize'

/**
 * 侧栏右边缘的拖拽手柄：拖动改变 --sidebar-width（聊天页/设置页同步）。
 * 仅在桌面（≥769px）可见可用；移动端侧栏是抽屉，固定 82vw。
 */
export function SidebarResizer() {
  const draggingRef = useRef(false)

  const onPointerMove = useCallback((e: PointerEvent) => {
    if (!draggingRef.current) return
    applySidebarWidth(e.clientX)
  }, [])

  const stopDrag = useCallback((e: PointerEvent) => {
    if (!draggingRef.current) return
    draggingRef.current = false
    document.body.classList.remove('sidebar-resizing')
    document.removeEventListener('pointermove', onPointerMove)
    document.removeEventListener('pointerup', stopDrag)
    document.removeEventListener('pointercancel', stopDrag)
    persistSidebarWidth(e.clientX)
  }, [onPointerMove])

  const startDrag = useCallback((e: React.PointerEvent<HTMLButtonElement>) => {
    // 抽屉断点内不允许拖拽
    if (window.matchMedia('(max-width: 768px)').matches) return
    e.preventDefault()
    draggingRef.current = true
    document.body.classList.add('sidebar-resizing')
    document.addEventListener('pointermove', onPointerMove)
    document.addEventListener('pointerup', stopDrag)
    document.addEventListener('pointercancel', stopDrag)
  }, [onPointerMove, stopDrag])

  useEffect(() => () => {
    document.body.classList.remove('sidebar-resizing')
    document.removeEventListener('pointermove', onPointerMove)
    document.removeEventListener('pointerup', stopDrag)
    document.removeEventListener('pointercancel', stopDrag)
  }, [onPointerMove, stopDrag])

  const nudge = useCallback((e: React.KeyboardEvent<HTMLButtonElement>) => {
    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()
    const current = parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--sidebar-width'))
    const next = (Number.isFinite(current) ? current : DEFAULT_SIDEBAR_WIDTH) + (e.key === 'ArrowRight' ? 12 : -12)
    persistSidebarWidth(applySidebarWidth(next))
  }, [])

  return (
    <button
      type="button"
      className="sidebar-resizer"
      aria-label="拖动调整侧栏宽度"
      aria-orientation="vertical"
      onPointerDown={startDrag}
      onKeyDown={nudge}
    />
  )
}
