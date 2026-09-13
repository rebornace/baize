import { describe, expect, it } from 'vitest'
import { conversationListLabel } from './conversationLabel'

describe('conversationListLabel', () => {
  it('prefixes weixin conversations', () => {
    expect(conversationListLabel('weixin:acc:peer1', '张三')).toBe('微信 · 张三')
  })

  it('uses default title when empty', () => {
    expect(conversationListLabel('weixin:acc:peer1', '  ')).toBe('微信 · 新对话')
  })

  it('leaves ui conversations unchanged', () => {
    expect(conversationListLabel('conv_abc', '你好')).toBe('你好')
    expect(conversationListLabel('conv_abc', '')).toBe('新对话')
  })
})
