import { afterEach, describe, expect, it } from 'vitest'
import { setPack } from '../locale/pack'
import { enPack } from '../locales/en'
import { zhPack } from '../locales/zh'
import { friendlyError, tierLabel } from '../strings'
import { ApiError } from '../api'
import { mainKnobFields } from '../pages/runtimeSettingsHelpers'

afterEach(() => {
  setPack(zhPack)
})

describe('helpers follow locale pack', () => {
  it('tierLabel and friendlyError switch with setPack', () => {
    expect(tierLabel('light')).toBe('快速')
    expect(friendlyError(new ApiError(401, 'unauthorized', 'x')).title).toContain('权限')

    setPack(enPack)
    expect(tierLabel('light')).toBe('Fast')
    expect(friendlyError(new ApiError(401, 'unauthorized', 'x')).title).toContain('access')
  })

  it('mainKnobFields labels follow setPack', () => {
    expect(mainKnobFields()[0].label).toBe(zhPack.RUNTIME.fieldMaxMessages)
    setPack(enPack)
    expect(mainKnobFields()[0].label).toBe(enPack.RUNTIME.fieldMaxMessages)
  })
})
