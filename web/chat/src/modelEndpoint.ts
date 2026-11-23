/**
 * Heuristic: treat common localhost / private / loopback base URLs as local
 * (or self-hosted). Everything else is treated as likely cloud for login risk copy.
 */
export function isLikelyLocalModelBaseURL(baseURL: string | undefined | null): boolean {
  const raw = (baseURL ?? '').trim().toLowerCase()
  if (!raw) return false
  let host = raw
  try {
    const u = new URL(raw.includes('://') ? raw : `http://${raw}`)
    host = u.hostname
  } catch {
    /* keep raw */
  }
  if (
    host === 'localhost' ||
    host === '127.0.0.1' ||
    host === '::1' ||
    host === '0.0.0.0' ||
    host.endsWith('.local')
  ) {
    return true
  }
  if (/^10\.\d+\.\d+\.\d+$/.test(host)) return true
  if (/^192\.168\.\d+\.\d+$/.test(host)) return true
  if (/^172\.(1[6-9]|2\d|3[0-1])\.\d+\.\d+$/.test(host)) return true
  return false
}

export function isLikelyCloudModelBaseURL(baseURL: string | undefined | null): boolean {
  return !isLikelyLocalModelBaseURL(baseURL)
}
