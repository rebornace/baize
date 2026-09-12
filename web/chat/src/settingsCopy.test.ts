import { describe, expect, it } from 'vitest'
import { driverLabel, identitySourceLabel, ACCOUNTS, STORAGE } from './strings'

describe('identitySourceLabel', () => {
  it('maps known sources to friendly Chinese', () => {
    expect(identitySourceLabel('login_capture')).toBe('对话中登录')
    expect(identitySourceLabel('env')).toBe('系统预设')
    expect(identitySourceLabel('manual')).toBe('临时提供')
  })
  it('falls back to the raw source for unknown values', () => {
    expect(identitySourceLabel('weird')).toBe('weird')
  })
})

describe('driverLabel', () => {
  it('maps storage drivers to friendly labels but keeps english submit value elsewhere', () => {
    expect(driverLabel('sqlite')).toBe('本地文件（SQLite）')
    expect(driverLabel('postgres')).toBe('PostgreSQL 数据库')
    expect(driverLabel('memory')).toBe('内存（重启即清空，仅试用）')
  })
  it('falls back to raw driver for unknown values', () => {
    expect(driverLabel('mysql')).toBe('mysql')
  })
})

describe('settings copy blocks exist', () => {
  it('exposes accounts/storage copy', () => {
    expect(ACCOUNTS.title).toBe('账号')
    expect(ACCOUNTS.emptyTitle).toContain('暂无')
    expect(STORAGE.title).toBe('数据存储')
    expect(STORAGE.confirmRestartTitle).toContain('重启')
  })
})
