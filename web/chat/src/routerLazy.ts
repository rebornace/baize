import { lazy, type ComponentType } from 'react'

/** Session-storage key guarding the one-shot reload for stale route chunks. */
export const ROUTE_RELOAD_KEY = 'baize.route-chunk-reload-at'
const RELOAD_WINDOW_MS = 30_000

type StorageLike = Pick<Storage, 'getItem' | 'setItem'>

/**
 * Loads a route module. A failed dynamic import usually means the tab still
 * points at an old build whose hashed chunk was removed by a deploy. Reload the
 * page once; if the chunk still fails, surface the error to the boundary.
 */
export async function loadRouteModule<T>(
  factory: () => Promise<T>,
  reload: () => void,
  now: () => number,
  storage: StorageLike | null,
): Promise<T> {
  try {
    return await factory()
  } catch (err) {
    let last = 0
    try {
      last = Number(storage?.getItem(ROUTE_RELOAD_KEY) || 0)
    } catch {
      last = 0
    }
    if (!last || now() - last > RELOAD_WINDOW_MS) {
      try {
        storage?.setItem(ROUTE_RELOAD_KEY, String(now()))
      } catch {
        /* storage can be unavailable in private mode */
      }
      reload()
    }
    throw err
  }
}

function safeSessionStorage(): StorageLike | null {
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

/** React.lazy with a one-shot reload for stale hashed chunks. */
export function lazyRoute<T extends ComponentType<any>>(
  factory: () => Promise<{ default: T }>,
) {
  return lazy(() =>
    loadRouteModule(factory, () => window.location.reload(), () => Date.now(), safeSessionStorage()),
  )
}
