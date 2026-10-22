/**
 * Open a URL outside the app shell (hosted login, channel files, etc.).
 * On Capacitor native, uses the Browser plugin so the system browser handles
 * cookies / redirects; on plain web, falls back to window.open.
 */
export async function openExternal(url: string): Promise<void> {
  const trimmed = url.trim()
  if (!trimmed) return

  try {
    const { Capacitor } = await import('@capacitor/core')
    if (Capacitor.isNativePlatform()) {
      const { Browser } = await import('@capacitor/browser')
      await Browser.open({ url: trimmed })
      return
    }
  } catch {
    /* Capacitor not linked (embedded /ui build) — use window.open */
  }

  window.open(trimmed, '_blank', 'noopener,noreferrer')
}
