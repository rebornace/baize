import { afterEach, describe, expect, it } from 'vitest'
import { setPack } from '../locale/pack'
import { enPack } from './en'
import { zhPack } from './zh'
import { ACTIONS, AUTO_LABEL, CHAT } from '../strings'

afterEach(() => {
  setPack(zhPack)
})

describe('live strings', () => {
  it('CHAT.copySuccess and AUTO_LABEL follow setPack', () => {
    expect(CHAT.copySuccess).toBe('已复制')
    expect(AUTO_LABEL).toBe('智能选择')
    expect(ACTIONS.copy).toBe('复制')

    setPack(enPack)
    expect(CHAT.copySuccess).toBe('Copied')
    expect(AUTO_LABEL).toBe('Auto select')
    expect(ACTIONS.copy).toBe('Copy')
  })
})
