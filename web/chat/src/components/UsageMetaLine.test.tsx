import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { UsageMeta } from '../foldEvents'
import { UsageMetaLine } from './UsageMetaLine'

function render(usage?: UsageMeta, durationMs?: number): string {
  return renderToStaticMarkup(createElement(UsageMetaLine, { usage, durationMs }))
}

describe('UsageMetaLine', () => {
  it('renders real usage line', () => {
    const html = render({
      promptTokens: 30,
      completionTokens: 10,
      totalTokens: 40,
      savedTokens: 0,
      cachedTokens: 0,
    })
    expect(html).toContain('msg-usage-details')
    expect(html).toContain('msg-usage')
    expect(html).toContain('用量 · 40')
    expect(html).toContain('输入 30')
    expect(html).toContain('输出 10')
    expect(html).toContain('共 40 tokens')
  })

  it('renders cache-hit breakdown inside the input count', () => {
    const html = render({
      promptTokens: 30,
      completionTokens: 10,
      totalTokens: 40,
      savedTokens: 0,
      cachedTokens: 23,
    })
    expect(html).toContain('输入 30')
    expect(html).toContain('缓存命中 23')
  })

  it('renders savings line', () => {
    const html = render({
      promptTokens: 0,
      completionTokens: 0,
      totalTokens: 0,
      savedTokens: 53,
      cachedTokens: 0,
    })
    expect(html).toContain('智能提速 · 约省 53')
    expect(html).toContain('约节省 53 tokens')
  })

  it('renders combined usage and savings', () => {
    const html = render({
      promptTokens: 30,
      completionTokens: 10,
      totalTokens: 40,
      savedTokens: 53,
      cachedTokens: 0,
    })
    expect(html).toContain('用量 · 40 · 约省 53')
    expect(html).toContain('共 40 tokens')
    expect(html).toContain('智能提速约节省 53 tokens')
  })

  it('renders duration without usage', () => {
    const html = render(undefined, 4500)
    expect(html).toContain('duration-line')
    expect(html).toContain('用时 4.5 秒')
    expect(html).not.toContain('msg-usage-details')
  })

  it('renders duration alongside usage', () => {
    const html = render(
      {
        promptTokens: 30,
        completionTokens: 10,
        totalTokens: 40,
        savedTokens: 0,
        cachedTokens: 0,
      },
      12_000,
    )
    expect(html).toContain('用时 12 秒')
    expect(html).toContain('用量 · 40')
  })

  it('renders nothing when there is nothing to report', () => {
    expect(
      render({ promptTokens: 0, completionTokens: 0, totalTokens: 0, savedTokens: 0, cachedTokens: 0 }),
    ).toBe('')
    expect(render()).toBe('')
  })
})
