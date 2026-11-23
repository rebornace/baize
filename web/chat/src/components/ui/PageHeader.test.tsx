import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { PageHeader } from './PageHeader'

describe('PageHeader', () => {
  it('renders title and description', () => {
    const html = renderToStaticMarkup(
      <PageHeader title="数据存储" description="选择数据保存位置" />,
    )
    expect(html).toContain('数据存储')
    expect(html).toContain('选择数据保存位置')
  })

  it('renders actions only when provided', () => {
    const withActions = renderToStaticMarkup(
      <PageHeader title="T" actions={<button type="button">刷新</button>} />,
    )
    expect(withActions).toContain('刷新')
    expect(withActions).toContain('ui-page-header-actions')
    const without = renderToStaticMarkup(<PageHeader title="T" />)
    expect(without).not.toContain('ui-page-header-actions')
  })
})
