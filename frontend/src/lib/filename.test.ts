import { describe, expect, it } from 'vitest'
import { shortName } from './filename'

describe('shortName', () => {
  it('leaves short names untouched', () => {
    expect(shortName('short.jpg', 20)).toBe('short.jpg')
  })

  it('middle-ellipses a long camera filename, keeping the extension', () => {
    expect(shortName('Screenshot_2026-07-12-21-31-45-really-long-name.jpg', 24)).toBe(
      'Screenshot…long-name.jpg',
    )
  })

  it('handles a name with no extension', () => {
    expect(shortName('noext-but-quite-a-long-name-here', 20)).toBe('noext-but-…name-here')
  })

  it('handles a dotfile (leading dot is not an extension separator)', () => {
    expect(shortName('.bashrc-long-dotfile-name', 15)).toBe('.bashrc…le-name')
  })
})
