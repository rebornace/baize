import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { UsageMeta } from '../foldEvents'
import { UsageMetaLine } from './UsageMetaLine'

function render(usage: UsageMeta): string {
  return renderToStaticMarkup(createElement(UsageMetaLine, { usage }))
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
    expect(html).toContain('msg-usage')
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
    expect(html).toContain('节省约 53 tokens')
    expect(html).toContain('智能提速为你节省')
  })

  it('renders combined usage and savings', () => {
    const html = render({
      promptTokens: 30,
      completionTokens: 10,
      totalTokens: 40,
      savedTokens: 53,
      cachedTokens: 0,
    })
    expect(html).toContain('共 40 tokens')
    expect(html).toContain('智能提速为你节省约 53 tokens')
  })

  it('renders nothing when there is nothing to report', () => {
    expect(
      render({ promptTokens: 0, completionTokens: 0, totalTokens: 0, savedTokens: 0, cachedTokens: 0 }),
    ).toBe('')
  })
})
