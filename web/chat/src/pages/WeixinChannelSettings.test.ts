import { describe, expect, it } from 'vitest'
import { setPack } from '../locale/pack'
import { zhPack } from '../locales/zh'
import { WEIXIN } from '../strings'
import {
  formatAllowlistText,
  loginStatusLabel,
  parseAllowlistText,
} from './WeixinChannelSettings'

setPack(zhPack)

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
    expect(loginStatusLabel('pending')).toBe(WEIXIN.loginPending)
    expect(loginStatusLabel('success')).toBe(WEIXIN.loginSuccess)
    expect(loginStatusLabel('expired')).toBe(WEIXIN.loginExpired)
  })
})
