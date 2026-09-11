import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { EmptyState } from './EmptyState'

describe('EmptyState', () => {
  it('renders title and description', () => {
    const html = renderToStaticMarkup(
      <EmptyState title="暂无已登录的业务账号" description="登录后会自动显示" />,
    )
    expect(html).toContain('暂无已登录的业务账号')
    expect(html).toContain('登录后会自动显示')
  })

  it('renders action and hides optional slots when absent', () => {
    const withAction = renderToStaticMarkup(
      <EmptyState title="空" action={<a href="/x">去添加</a>} />,
    )
    expect(withAction).toContain('去添加')
    expect(withAction).toContain('ui-empty-state-action')
    const plain = renderToStaticMarkup(<EmptyState title="空" />)
    expect(plain).not.toContain('ui-empty-state-action')
    expect(plain).not.toContain('ui-empty-state-icon')
  })
})
