import { describe, expect, it } from 'vitest'
import { isLikelyCloudModelBaseURL, isLikelyLocalModelBaseURL } from './modelEndpoint'

describe('modelEndpoint heuristics', () => {
  it('treats localhost and private nets as local', () => {
    expect(isLikelyLocalModelBaseURL('http://localhost:11434')).toBe(true)
    expect(isLikelyLocalModelBaseURL('http://127.0.0.1:8080/v1')).toBe(true)
    expect(isLikelyLocalModelBaseURL('http://10.0.0.5:8000')).toBe(true)
    expect(isLikelyLocalModelBaseURL('http://192.168.1.10')).toBe(true)
    expect(isLikelyLocalModelBaseURL('http://172.16.0.2')).toBe(true)
    expect(isLikelyCloudModelBaseURL('http://localhost:11434')).toBe(false)
  })

  it('treats public hosts as cloud', () => {
    expect(isLikelyCloudModelBaseURL('https://api.openai.com/v1')).toBe(true)
    expect(isLikelyLocalModelBaseURL('https://api.openai.com/v1')).toBe(false)
    expect(isLikelyCloudModelBaseURL('')).toBe(true)
  })
})
