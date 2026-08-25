import { describe, expect, it } from 'vitest'
import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { Composer } from './Composer'
import type { SkillSummary } from '../api'

const skills: SkillSummary[] = [
  { id: 'search', name: 'Search', description: '搜索知识库', tools: [], source: 'builtin' },
  { id: 'sum', name: 'Sum', description: '求和', tools: [], source: 'user' },
]

describe('Composer', () => {
  it('renders textarea, a visible attachment button and a disabled send button when empty', () => {
    const html = renderToStaticMarkup(createElement(Composer, { onSend: () => {} }))
    expect(html).toContain('aria-label="消息输入"')
    // The visible attachment button must carry the accessible label and the
    // composer-attach class (the hidden file input is aria-hidden / tabIndex=-1).
    expect(html).toContain('composer-attach')
    expect(html).toContain('aria-label="添加附件"')
    expect(html).toContain('disabled=""')
    expect(html).toContain('发送')
  })

  it('hides the raw file input from assistive tech and tabs (button triggers it)', () => {
    const html = renderToStaticMarkup(createElement(Composer, { onSend: () => {} }))
    expect(html).toContain('aria-hidden="true"')
    expect(html).toContain('tabindex="-1"')
    // The hidden input must not carry the visible label (button owns it).
    const inputMatch = html.match(/<input[^>]*composer-file-input[^>]*>/)
    expect(inputMatch).toBeTruthy()
    expect(inputMatch![0]).not.toContain('aria-label="添加附件"')
  })

  it('lists available skill ids as a hint when skills are provided', () => {
    const html = renderToStaticMarkup(
      createElement(Composer, { onSend: () => {}, skills }),
    )
    expect(html).toContain('composer-hint')
    expect(html).toContain('search')
    expect(html).toContain('sum')
  })

  it('omits the skill hint when no skills are available', () => {
    const html = renderToStaticMarkup(
      createElement(Composer, { onSend: () => {}, skills: [] }),
    )
    expect(html).not.toContain('composer-hint')
  })

  it('disables the attachment button and inputs when disabled prop is set', () => {
    const html = renderToStaticMarkup(
      createElement(Composer, { onSend: () => {}, disabled: true }),
    )
    // attachment button, file input, textarea and send button all carry disabled
    const disabledCount = html.match(/disabled=""/g)?.length ?? 0
    expect(disabledCount).toBeGreaterThanOrEqual(3)
    // The attachment button specifically must be disabled.
    const attachBtn = html.match(/<button[^>]*composer-attach[^>]*>/)
    expect(attachBtn).toBeTruthy()
    expect(attachBtn![0]).toContain('disabled=""')
  })
})
