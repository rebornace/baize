import { authHeaders } from '../controlAuth'

/** Only persisted conversation media can be deleted server-side. */
export function isDeletableMediaUrl(url: string): boolean {
  return url.startsWith('/v0/channels/media/')
}

/**
 * Deletes a persisted attachment. When `name` is known, the server also removes
 * the workspace copy saved under that original filename. Returns true when the
 * attachment is gone (a 404 counts: it was already removed).
 */
export async function deleteChannelMedia(
  url: string,
  name: string,
  gateEnabled: boolean,
): Promise<boolean> {
  if (!isDeletableMediaUrl(url)) return false
  const hasName = /[?&]name=/.test(url)
  const target =
    name && !hasName
      ? `${url}${url.includes('?') ? '&' : '?'}name=${encodeURIComponent(name)}`
      : url
  try {
    const res = await fetch(target, {
      method: 'DELETE',
      headers: authHeaders(gateEnabled),
    })
    return res.ok || res.status === 404
  } catch {
    return false
  }
}
