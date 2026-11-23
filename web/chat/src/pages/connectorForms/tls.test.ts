import { describe, expect, it } from 'vitest'
import { tlsEmpty, tlsForPut, tlsFromConnector, tlsPayloadForSave, validateTls } from './tls'

describe('connector TLS helpers', () => {
  it('tlsEmpty treats blank paths as empty', () => {
    expect(tlsEmpty({})).toBe(true)
    expect(tlsEmpty({ cert_file: ' ', key_file: '', ca_file: '' })).toBe(true)
    expect(tlsEmpty({ ca_file: '/ca.pem' })).toBe(false)
  })

  it('validateTls requires cert/key pair', () => {
    expect(validateTls({ cert_file: 'a.crt' }, 'pair')).toEqual({ pair: 'pair' })
    expect(validateTls({ key_file: 'a.key' }, 'pair')).toEqual({ pair: 'pair' })
    expect(validateTls({ cert_file: 'a.crt', key_file: 'a.key' }, 'pair')).toBeNull()
    expect(validateTls({ ca_file: 'ca.pem' }, 'pair')).toBeNull()
  })

  it('tlsForPut trims and drops blanks', () => {
    expect(tlsForPut({ cert_file: ' a.crt ', key_file: ' a.key ', ca_file: ' ' })).toEqual({
      cert_file: 'a.crt',
      key_file: 'a.key',
    })
    expect(tlsFromConnector({ cert_file: '/c', key_file: '/k' })).toEqual({
      cert_file: '/c',
      key_file: '/k',
      ca_file: '',
    })
  })

  it('tlsPayloadForSave omits empty unless explicitly cleared', () => {
    expect(tlsPayloadForSave({}, false)).toBeUndefined()
    expect(tlsPayloadForSave({}, true)).toEqual({})
    expect(tlsPayloadForSave({ ca_file: '/ca.pem' }, false)).toEqual({ ca_file: '/ca.pem' })
  })
})
