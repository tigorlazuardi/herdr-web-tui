import { describe, expect, it } from 'vitest'
import { sessionFromPath } from './session'

describe('sessionFromPath', () => {
  it('extracts the first path segment as the session name', () => {
    expect(sessionFromPath('/work')).toBe('work')
    expect(sessionFromPath('/foo-bar-99')).toBe('foo-bar-99')
  })

  it('takes only the first segment when the path has more', () => {
    expect(sessionFromPath('/work/nested')).toBe('work')
  })

  it('returns empty string for root, letting the caller fall back to default', () => {
    expect(sessionFromPath('/')).toBe('')
    expect(sessionFromPath('')).toBe('')
  })
})
