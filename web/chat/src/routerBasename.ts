/** React Router / logout home path. Embedded server uses `/ui`; Capacitor shell uses `/`. */
export function routerBasename(): string {
  const raw = (import.meta.env.VITE_ROUTER_BASENAME as string | undefined)?.trim()
  // Unset/empty → embedded `/ui`. Explicit `/` or `.` → Capacitor root (empty basename).
  if (!raw) return '/ui'
  if (raw === '/' || raw === '.') return ''
  return raw.replace(/\/$/, '')
}

export function uiHomeURL(): string {
  const base = routerBasename()
  return base ? `${base}/` : '/'
}
