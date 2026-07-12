import { describe, expect, it } from 'vitest'
import { parseTemplate, removeMarker, type Attachment } from './markers'

function attachment(id: string, name: string, mime = 'image/png'): Attachment {
  return { id, mime, file: new File(['x'], name, { type: mime }) }
}

describe('parseTemplate', () => {
  it('returns a single text segment when there are no markers', () => {
    expect(parseTemplate('hello world', [])).toEqual([{ kind: 'text', text: 'hello world' }])
  })

  it('returns no segments for empty text', () => {
    expect(parseTemplate('', [])).toEqual([])
  })

  it('splits a single marker into text/pill/text', () => {
    const a = attachment('a', 'shot.png')
    expect(parseTemplate('see §1 now', [a])).toEqual([
      { kind: 'text', text: 'see ' },
      { kind: 'pill', id: 'a', file: a.file, mime: a.mime },
      { kind: 'text', text: ' now' },
    ])
  })

  it('handles multiple markers in order', () => {
    const a = attachment('a', 'one.png')
    const b = attachment('b', 'two.pdf', 'application/pdf')
    expect(parseTemplate('§1 and §2', [a, b])).toEqual([
      { kind: 'pill', id: 'a', file: a.file, mime: a.mime },
      { kind: 'text', text: ' and ' },
      { kind: 'pill', id: 'b', file: b.file, mime: b.mime },
    ])
  })

  it('leaves an out-of-range marker as literal text', () => {
    const a = attachment('a', 'one.png')
    expect(parseTemplate('§1 and §9', [a])).toEqual([
      { kind: 'pill', id: 'a', file: a.file, mime: a.mime },
      { kind: 'text', text: ' and §9' },
    ])
  })

  it('leaves every marker as literal text when there are no attachments', () => {
    expect(parseTemplate('mid-typing §', [])).toEqual([{ kind: 'text', text: 'mid-typing §' }])
  })

  it('allows a duplicate reference to the same attachment', () => {
    const a = attachment('a', 'one.png')
    expect(parseTemplate('§1 twice §1', [a])).toEqual([
      { kind: 'pill', id: 'a', file: a.file, mime: a.mime },
      { kind: 'text', text: ' twice ' },
      { kind: 'pill', id: 'a', file: a.file, mime: a.mime },
    ])
  })
})

describe('removeMarker', () => {
  it('removes the token and renumbers surviving markers', () => {
    const a = attachment('a', 'one.png')
    const b = attachment('b', 'two.png')
    const c = attachment('c', 'three.png')
    const result = removeMarker('a §1 b §2 c §3', [a, b, c], 2)

    expect(result.text).toBe('a §1 b  c §2')
    expect(result.attachments).toEqual([a, c])
  })

  it('is a no-op for an out-of-range index', () => {
    const a = attachment('a', 'one.png')
    const result = removeMarker('a §1', [a], 5)
    expect(result).toEqual({ text: 'a §1', attachments: [a] })
  })

  it('removes all occurrences of a duplicated reference', () => {
    const a = attachment('a', 'one.png')
    const b = attachment('b', 'two.png')
    const result = removeMarker('§1 x §1 y §2', [a, b], 1)

    expect(result.text).toBe(' x  y §1')
    expect(result.attachments).toEqual([b])
  })

  it('leaves lower-numbered markers untouched when removing the last attachment', () => {
    const a = attachment('a', 'one.png')
    const b = attachment('b', 'two.png')
    const result = removeMarker('§1 §2', [a, b], 2)

    expect(result.text).toBe('§1 ')
    expect(result.attachments).toEqual([a])
  })
})
