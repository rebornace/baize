import { useCallback, useState } from 'react'

/** 移动端抽屉的开关状态；初始关闭。 */
export function useDrawer(initial = false) {
  const [isOpen, setOpen] = useState(initial)
  return {
    isOpen,
    open: useCallback(() => setOpen(true), []),
    close: useCallback(() => setOpen(false), []),
    toggle: useCallback(() => setOpen((v) => !v), []),
  }
}
