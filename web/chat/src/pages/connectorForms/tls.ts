import type { ConnectorTLS } from '../../api'

export type TlsFieldErrors = Partial<Record<'pair', string>>

export function tlsFromConnector(tls?: ConnectorTLS | null): ConnectorTLS {
  return {
    cert_file: tls?.cert_file?.trim() ?? '',
    key_file: tls?.key_file?.trim() ?? '',
    ca_file: tls?.ca_file?.trim() ?? '',
  }
}

export function tlsEmpty(t: ConnectorTLS): boolean {
  return !t.cert_file?.trim() && !t.key_file?.trim() && !t.ca_file?.trim()
}

/** cert/key must both be set or both empty; ca alone is ok. */
export function validateTls(t: ConnectorTLS, pairError: string): TlsFieldErrors | null {
  const cert = t.cert_file?.trim() ?? ''
  const key = t.key_file?.trim() ?? ''
  if ((cert === '') !== (key === '')) {
    return { pair: pairError }
  }
  return null
}

/** Normalized body for PUT (trimmed; empty strings omitted via omitempty on wire as empty object fields). */
export function tlsForPut(t: ConnectorTLS): ConnectorTLS {
  const out: ConnectorTLS = {}
  const cert = t.cert_file?.trim() ?? ''
  const key = t.key_file?.trim() ?? ''
  const ca = t.ca_file?.trim() ?? ''
  if (cert) out.cert_file = cert
  if (key) out.key_file = key
  if (ca) out.ca_file = ca
  return out
}

/**
 * PUT semantics: non-empty → send tls; all empty + explicit clear → send {};
 * all empty without clear → omit (preserve server value).
 */
export function tlsPayloadForSave(t: ConnectorTLS, cleared: boolean): ConnectorTLS | undefined {
  if (!tlsEmpty(t)) return tlsForPut(t)
  if (cleared) return {}
  return undefined
}
