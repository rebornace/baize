import { describe, expect, it } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { MarkdownText } from './MarkdownText'

describe('MarkdownText', () => {
  it('renders bold and code', () => {
    const html = renderToStaticMarkup(
      createElement(MarkdownText, { text: '你好 **世界** 和 `code`' }),
    )
    expect(html).toContain('<strong>世界</strong>')
    expect(html).toContain('md-code')
    expect(html).toContain('code')
  })

  it('renders fenced code', () => {
    const html = renderToStaticMarkup(
      createElement(MarkdownText, { text: '```\nconst a = 1\n```' }),
    )
    expect(html).toContain('md-pre')
    expect(html).toContain('const a = 1')
  })
})
