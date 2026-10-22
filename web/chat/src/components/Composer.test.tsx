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
  it('renders textarea, a plus menu trigger and a disabled send button when empty', () => {
    const html = renderToStaticMarkup(createElement(Composer, { onSend: () => {} }))
    expect(html).toContain('aria-label="消息输入"')
    expect(html).toContain('composer-attach')
    expect(html).toContain('aria-label="添加"')
    expect(html).toContain('disabled=""')
    expect(html).toContain('发送')
  })

  it('hides the raw file input from assistive tech and tabs (button triggers it)', () => {
    const html = renderToStaticMarkup(createElement(Composer, { onSend: () => {} }))
    expect(html).toContain('aria-hidden="true"')
    expect(html).toContain('tabindex="-1"')
    const inputMatch = html.match(/<input[^>]*composer-file-input[^>]*>/)
    expect(inputMatch).toBeTruthy()
    expect(inputMatch![0]).not.toContain('aria-label="添加"')
  })

  it('lists available skill ids and reload hint when skills are provided', () => {
    const html = renderToStaticMarkup(
      createElement(Composer, { onSend: () => {}, skills }),
    )
    expect(html).toContain('composer-hint')
    expect(html).toContain('search')
    expect(html).toContain('sum')
    expect(html).toContain('/reload')
  })

  it('still shows the reload hint when the skill list is empty', () => {
    const html = renderToStaticMarkup(
      createElement(Composer, { onSend: () => {}, skills: [] }),
    )
    expect(html).toContain('composer-hint')
    expect(html).toContain('/reload')
  })

  it('omits the hint when skills prop is omitted', () => {
    const html = renderToStaticMarkup(createElement(Composer, { onSend: () => {} }))
    expect(html).not.toContain('composer-hint')
  })

  it('disables the plus trigger and inputs when disabled prop is set', () => {
    const html = renderToStaticMarkup(
      createElement(Composer, { onSend: () => {}, disabled: true }),
    )
    const disabledCount = html.match(/disabled=""/g)?.length ?? 0
    expect(disabledCount).toBeGreaterThanOrEqual(3)
    const attachBtn = html.match(/<button[^>]*composer-attach[^>]*>/)
    expect(attachBtn).toBeTruthy()
    expect(attachBtn![0]).toContain('disabled=""')
  })

  it('renders the trailing slot inside composer-box when provided', () => {
    const html = renderToStaticMarkup(
      createElement(Composer, {
        onSend: () => {},
        trailing: createElement('span', { 'data-testid': 'tr' }, '模型'),
      }),
    )
    expect(html).toContain('composer-trailing')
    expect(html).toContain('模型')
    expect(html.indexOf('composer-trailing')).toBeGreaterThan(html.indexOf('composer-attach'))
    expect(html.indexOf('composer-trailing')).toBeLessThan(html.indexOf('composer-send'))
  })

  it('omits the trailing slot when not provided', () => {
    const html = renderToStaticMarkup(createElement(Composer, { onSend: () => {} }))
    expect(html).not.toContain('composer-trailing')
  })
})
