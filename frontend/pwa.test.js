import { readFileSync } from 'node:fs'
import { describe, expect, test } from 'vitest'

const publicFile = (name) => readFileSync(new URL(`./public/${name}`, import.meta.url))
const pngSize = (name) => {
  const png = publicFile(name)
  return [png.readUInt32BE(16), png.readUInt32BE(20)]
}

describe('PWA install metadata', () => {
  test('references valid Android icon sizes', () => {
    const manifest = JSON.parse(publicFile('manifest.webmanifest').toString())

    expect(manifest.display).toBe('standalone')
    expect(manifest.icons.map(({ src, sizes }) => [src, sizes])).toEqual([
      ['/icon-192.png', '192x192'],
      ['/icon-512.png', '512x512'],
    ])
    expect(pngSize('icon-192.png')).toEqual([192, 192])
    expect(pngSize('icon-512.png')).toEqual([512, 512])
  })
})
