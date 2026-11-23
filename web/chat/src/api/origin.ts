/** Optional absolute origin for API calls when the UI is not same-origin (Capacitor / mobile shell). */
export function apiOrigin(): string {
  const raw = (import.meta.env.VITE_API_ORIGIN as string | undefined)?.trim() ?? ''
  return raw.replace(/\/$/, '')
}

/** Prefix a root-relative path (`/v0/...`) with {@link apiOrigin} when set. */
export function apiURL(path: string): string {
  if (!path.startsWith('/')) return path
  const origin = apiOrigin()
  return origin ? `${origin}${path}` : path
}

/**
 * Rewrite root-relative `fetch('/v0/...')` to the configured API origin.
 * No-op when `VITE_API_ORIGIN` is empty (embedded `/ui` on the same host).
 */
export function installApiOrigin(): void {
  const origin = apiOrigin()
  if (!origin || typeof window === 'undefined') return

  const nativeFetch = window.fetch.bind(window)
  window.fetch = (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    if (typeof input === 'string' && input.startsWith('/')) {
      return nativeFetch(`${origin}${input}`, init)
    }
    if (input instanceof URL) {
      if (input.origin === window.location.origin && input.pathname.startsWith('/v0')) {
        return nativeFetch(`${origin}${input.pathname}${input.search}${input.hash}`, init)
      }
      return nativeFetch(input, init)
    }
    if (input instanceof Request) {
      try {
        const u = new URL(input.url)
        if (u.origin === window.location.origin && u.pathname.startsWith('/v0')) {
          return nativeFetch(new Request(`${origin}${u.pathname}${u.search}${u.hash}`, input), init)
        }
      } catch {
        /* fall through */
      }
    }
    return nativeFetch(input, init)
  }
}
