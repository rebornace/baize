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
  it('renders textarea, file input and a disabled send button when empty', () => {
    const html = renderToStaticMarkup(createElement(Composer, { onSend: () => {} }))
    expect(html).toContain('aria-label="消息输入"')
    expect(html).toContain('aria-label="添加附件"')
    expect(html).toContain('disabled=""')
    expect(html).toContain('发送')
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

  it('disables inputs when disabled prop is set', () => {
    const html = renderToStaticMarkup(
      createElement(Composer, { onSend: () => {}, disabled: true }),
    )
    expect(html).toContain('aria-label="添加附件"')
    // both textarea and file input carry the disabled attribute
    expect(html.match(/disabled=""/g)?.length ?? 0).toBeGreaterThanOrEqual(2)
  })
})
