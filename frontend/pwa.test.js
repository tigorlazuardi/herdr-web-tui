import { readFileSync } from 'node:fs'
import { describe, expect, test } from 'vitest'

const publicFile = (name) => readFileSync(new URL(`./public/${name}`, import.meta.url))
const pngSize = (name) => {
  const png = publicFile(name)
  return [png.readUInt32BE(16), png.readUInt32BE(20)]
}

describe('PWA install metadata', () => {
  test('sends auth credentials when loading the protected manifest', () => {
    const html = readFileSync(new URL('./index.html', import.meta.url), 'utf8')
    expect(html).toContain('rel="manifest" href="/manifest.webmanifest" crossorigin="use-credentials"')
  })

  test('ships valid default icon sizes', () => {
    expect(pngSize('favicon.png')).toEqual([32, 32])
    expect(pngSize('icon-192.png')).toEqual([192, 192])
    expect(pngSize('icon-512.png')).toEqual([512, 512])
  })
})
