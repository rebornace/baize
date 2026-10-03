import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { Card } from './Card'

describe('Card', () => {
  it('renders title, description and body content', () => {
    const html = renderToStaticMarkup(
      <Card title="AI 模型" description="选择对话用的大脑">
        <span>body</span>
      </Card>,
    )
    expect(html).toContain('data-testid="ui-card"')
    expect(html).toContain('AI 模型')
    expect(html).toContain('选择对话用的大脑')
    expect(html).toContain('body')
  })
})
