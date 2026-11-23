import { describe, expect, it } from 'vitest'
import { qrDataUrlFromText } from './qrDataUrl'

describe('qrDataUrlFromText', () => {
  it('encodes a liteapp URL as a PNG data URL', async () => {
    const url =
      'https://liteapp.weixin.qq.com/q/7GiQu1?qrcode=9292594bd8f866640caaab428aa43b81&bot_type=3'
    const dataUrl = await qrDataUrlFromText(url)
    expect(dataUrl.startsWith('data:image/png;base64,')).toBe(true)
    expect(dataUrl.length).toBeGreaterThan(100)
  })

  it('rejects empty payload', async () => {
    await expect(qrDataUrlFromText('   ')).rejects.toThrow(/empty/)
  })
})
