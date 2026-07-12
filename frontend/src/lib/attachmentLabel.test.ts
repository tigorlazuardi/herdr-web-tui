import { describe, expect, it } from 'vitest'
import { attachmentLabel, labelAttachments } from './attachmentLabel'

describe('attachmentLabel', () => {
  it('labels images generically', () => {
    expect(attachmentLabel('image/jpeg')).toBe('Image')
    expect(attachmentLabel('image/png')).toBe('Image')
  })

  it('labels pdf', () => {
    expect(attachmentLabel('application/pdf')).toBe('PDF')
  })

  it('labels text', () => {
    expect(attachmentLabel('text/plain')).toBe('Text')
  })

  it('labels archives', () => {
    expect(attachmentLabel('application/zip')).toBe('Archive')
    expect(attachmentLabel('application/gzip')).toBe('Archive')
    expect(attachmentLabel('application/x-tar')).toBe('Archive')
  })

  it('falls back to File for anything unmapped', () => {
    expect(attachmentLabel('application/octet-stream')).toBe('File')
  })
})

describe('labelAttachments', () => {
  it('leaves a lone attachment of a kind unindexed', () => {
    expect(labelAttachments(['image/jpeg', 'application/pdf'])).toEqual(['Image', 'PDF'])
  })

  it('indexes attachments that share a label, in order', () => {
    expect(labelAttachments(['image/jpeg', 'application/pdf', 'image/png'])).toEqual([
      'Image 1',
      'PDF',
      'Image 2',
    ])
  })

  it('indexes every duplicate group independently', () => {
    expect(labelAttachments(['application/zip', 'application/x-tar', 'text/plain'])).toEqual([
      'Archive 1',
      'Archive 2',
      'Text',
    ])
  })
})
