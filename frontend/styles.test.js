import { describe, expect, it } from 'vitest'
import fs from 'node:fs'

it('declares bundled supplementary Nerd PUA range', () => {
  const css = fs.readFileSync(new URL('./src/styles.css', import.meta.url), 'utf8')
  expect(css).toMatch(/unicode-range:\s*U\+E000-F8FF,\s*U\+F0001-F1AF0/)
})
