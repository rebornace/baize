import { describe, expect, it } from 'vitest'
import {
  formatAllowlistText,
  loginStatusLabel,
  parseAllowlistText,
} from './WeixinChannelSettings'

describe('parseAllowlistText', () => {
  it('splits lines and trims empties', () => {
    expect(parseAllowlistText(' a \n\nb\n  ')).toEqual(['a', 'b'])
  })

  it('returns empty for blank', () => {
    expect(parseAllowlistText('  \n  ')).toEqual([])
  })
})

describe('formatAllowlistText', () => {
  it('joins with newlines', () => {
    expect(formatAllowlistText(['a', 'b'])).toBe('a\nb')
  })

  it('handles undefined', () => {
    expect(formatAllowlistText(undefined)).toBe('')
  })
})

describe('loginStatusLabel', () => {
  it('maps known statuses', () => {
    expect(loginStatusLabel('pending')).toContain('扫码')
    expect(loginStatusLabel('success')).toContain('成功')
    expect(loginStatusLabel('expired')).toContain('过期')
  })
})
