// Visual-regression driver for mobile-ux-v2.md's three fixes. Runs inside
// the playwright podman image (see run-visual.sh) against the real app
// server on the host, using --network=host. Plain script, no test runner:
// this is a screenshot tool for a human to look at, not a pass/fail suite.
import { chromium, devices } from 'playwright'
import { mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join, dirname } from 'node:path'
import { fileURLToPath } from 'node:url'

const APP_URL = 'http://127.0.0.1:8099/'
const OUT = join(dirname(fileURLToPath(import.meta.url)), 'screens')

async function main() {
  const browser = await chromium.launch()
  const context = await browser.newContext({ ...devices['Pixel 7'] })
  const page = await context.newPage()

  await page.goto(APP_URL)
  // Terminal renders black in headless (no real interactive pty session
  // reaches a paintable state without a TTY) — expected, we're only
  // checking UI chrome (topbar, promptbox, key bar), not terminal output.
  await page.waitForTimeout(2500)
  await page.screenshot({ path: join(OUT, '01-baseline.png') })

  // A deliberately long, realistic screenshot filename — the promptbox
  // never displays it (attachments are generic thumbnails + a §N marker
  // token, not filenames), this is just realistic input.
  const dir = mkdtempSync(join(tmpdir(), 'herdr-e2e-'))
  const longName = 'Screenshot_2026-07-12-21-31-45-really-long-name.jpg'
  const filePath = join(dir, longName)
  writeFileSync(filePath, Buffer.from([0xff, 0xd8, 0xff, 0xd9])) // minimal jpeg-ish bytes, content unused

  // Default mode is promptbox (mobile-ux-v2.md exclusive-mode rework): no
  // keybar toggle needed to see it. Attaching a file inserts a §1 marker
  // token into the textarea and a thumbnail into the strip above it, so
  // the promptbox screenshot isn't just an empty box.
  await page.setInputFiles('input[type=file]', filePath)
  await page.waitForTimeout(300)
  await page.screenshot({ path: join(OUT, '02-promptbox.png') })

  // Topbar toggle is the rightmost control (mobile-ux-v2.md "Topbar").
  // Promptbox and keybar are mutually exclusive: tapping it swaps the
  // promptbox out for the accessory key bar, never both on screen.
  await page.click('button[aria-label="Switch to keys"]')
  await page.waitForTimeout(300)
  await page.screenshot({ path: join(OUT, '03-keys.png') })

  await browser.close()
  console.log('done')
}

main().catch((err) => {
  console.error(err)
  process.exit(1)
})
