const STORAGE_KEY = 'baize.desktopNotify'

export function notificationsSupported(): boolean {
  return typeof window !== 'undefined' && 'Notification' in window
}

export function desktopNotifyEnabled(): boolean {
  try {
    return localStorage.getItem(STORAGE_KEY) === '1'
  } catch {
    return false
  }
}

export function setDesktopNotifyEnabled(on: boolean): void {
  try {
    if (on) localStorage.setItem(STORAGE_KEY, '1')
    else localStorage.removeItem(STORAGE_KEY)
  } catch {
    // Private mode / disabled storage: notifications stay off for this session.
  }
}

export async function requestDesktopNotifyPermission(): Promise<NotificationPermission> {
  if (!notificationsSupported()) return 'denied'
  if (Notification.permission === 'granted') return 'granted'
  if (Notification.permission === 'denied') return 'denied'
  try {
    return await Notification.requestPermission()
  } catch {
    return 'denied'
  }
}

/** Show one tagged notification; returns false when unsupported or unpermitted. */
export function showPendingNotification(
  title: string,
  body: string,
  onClick: () => void,
): boolean {
  if (!notificationsSupported() || Notification.permission !== 'granted') return false
  try {
    const note = new Notification(title, { body, tag: 'baize-pending' })
    note.onclick = () => {
      try {
        window.focus()
      } catch {
        // Some embedded webviews cannot focus themselves.
      }
      note.close()
      onClick()
    }
    return true
  } catch {
    return false
  }
}
