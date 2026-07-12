import { describe, expect, it } from 'vitest'
import { serialize, type PillSegment, type Segment, type TextSegment } from './segments'

function textSeg(text: string): TextSegment {
  return { kind: 'text', text }
}

function pillSeg(id: string, name: string, mime = 'image/png'): PillSegment {
  return { kind: 'pill', id, mime, file: new File(['x'], name, { type: mime }) }
}

describe('serialize', () => {
  it('handles an empty segment list', () => {
    const { template, files } = serialize([])
    expect(template).toEqual({ segments: [] })
    expect(files).toEqual([])
  })

  it('serializes text-only segments verbatim, in order', () => {
    const { template, files } = serialize([textSeg('imgcat '), textSeg('now')])
    expect(template.segments).toEqual([{ text: 'imgcat ' }, { text: 'now' }])
    expect(files).toEqual([])
  })

  it('serializes a pill-only segment to a file marker with a generated field name', () => {
    const pill = pillSeg('a', 'shot.png')
    const { template, files } = serialize([pill])
    expect(template.segments).toEqual([{ file: 'file0' }])
    expect(files).toHaveLength(1)
    expect(files[0]).toEqual({ fieldName: 'file0', file: pill.file })
  })

  it('preserves caret order for a mixed text/pill/text sequence', () => {
    const pill = pillSeg('a', 'shot.png')
    const segments = [textSeg('imgcat '), pill, textSeg(' now')]
    const { template, files } = serialize(segments)
    expect(template.segments).toEqual([{ text: 'imgcat ' }, { file: 'file0' }, { text: ' now' }])
    expect(files).toEqual([{ fieldName: 'file0', file: pill.file }])
  })

  it('assigns field names by pill position, not text position, for consecutive pills', () => {
    const p1 = pillSeg('a', 'one.png')
    const p2 = pillSeg('b', 'two.pdf', 'application/pdf')
    const { template, files } = serialize([p1, p2, textSeg(' tail')])
    expect(template.segments).toEqual([{ file: 'file0' }, { file: 'file1' }, { text: ' tail' }])
    expect(files.map((f) => f.fieldName)).toEqual(['file0', 'file1'])
    expect(files[0].file).toBe(p1.file)
    expect(files[1].file).toBe(p2.file)
  })

  it('round-trips an empty text run without dropping it', () => {
    const { template } = serialize([textSeg('')])
    expect(template.segments).toEqual([{ text: '' }])
  })

  it('never emits both text and file on one wire segment', () => {
    const { template } = serialize([textSeg('a'), pillSeg('a', 'x.txt', 'text/plain')])
    for (const seg of template.segments) {
      expect(seg.text === undefined || seg.file === undefined).toBe(true)
    }
  })
})
