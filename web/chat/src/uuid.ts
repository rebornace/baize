// uuid returns an RFC 4122 version 4 UUID.
//
// crypto.randomUUID() exists only in secure contexts (HTTPS or localhost /
// 127.0.0.1). Opening the UI over a plain-HTTP LAN address such as
// http://192.168.1.55:8080 is an insecure context, where randomUUID is
// undefined and calling it throws on first render. crypto.getRandomValues() is
// available even in insecure contexts, so fall back to a v4 generator built on
// it. Both paths produce the same canonical 8-4-4-4-12 hex string.
export function uuid(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  // RFC 4122 §4.4: set version (4) and variant (10xx) bits.
  bytes[6] = (bytes[6] & 0x0f) | 0x40
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex: string[] = []
  for (const b of bytes) hex.push(b.toString(16).padStart(2, '0'))
  return (
    hex.slice(0, 4).join('') +
    '-' +
    hex.slice(4, 6).join('') +
    '-' +
    hex.slice(6, 8).join('') +
    '-' +
    hex.slice(8, 10).join('') +
    '-' +
    hex.slice(10, 16).join('')
  )
}
