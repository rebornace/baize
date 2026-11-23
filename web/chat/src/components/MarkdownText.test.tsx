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

  it('rewrites bare artifact links to human label', () => {
    const html = renderToStaticMarkup(
      createElement(MarkdownText, {
        text: '见 [/v0/artifacts/art_1](/v0/artifacts/art_1)',
      }),
    )
    expect(html).toContain('打开分析页')
    expect(html).not.toContain('/v0/artifacts/art_1">/v0/artifacts')
  })

  it('renders fenced code', () => {
    const html = renderToStaticMarkup(
      createElement(MarkdownText, { text: '```\nconst a = 1\n```' }),
    )
    expect(html).toContain('md-pre')
    expect(html).toContain('const a = 1')
  })

  it('renders markdown headings with chat-friendly levels', () => {
    const html = renderToStaticMarkup(
      createElement(MarkdownText, { text: '# Section\n\n## Detail\n\n### Note' }),
    )
    expect(html).toContain('<h3 class="md-heading">Section</h3>')
    expect(html).toContain('<h4 class="md-heading">Detail</h4>')
    expect(html).toContain('<h5 class="md-heading">Note</h5>')
  })
})
