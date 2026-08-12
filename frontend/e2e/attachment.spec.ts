import { expect, test } from '@playwright/test'

test('selecting a file inserts its template marker without crypto.randomUUID', async ({ page }) => {
  await page.addInitScript(() => Object.defineProperty(crypto, 'randomUUID', { value: undefined }))
  await page.goto('/')
  await page.locator('input[type="file"]').setInputFiles({
    name: 'probe.txt',
    mimeType: 'text/plain',
    buffer: Buffer.from('probe'),
  })

  await expect(page.getByRole('textbox', { name: 'Message' })).toHaveValue('§1')
  await expect(page.getByText('§1', { exact: true })).toBeVisible()
})

test('composer controls align and textarea grows to at most three lines', async ({ page }) => {
  await page.goto('/')

  const textarea = page.getByRole('textbox', { name: 'Message' })
  const attach = page.getByRole('button', { name: 'Attach a file' })
  const send = page.getByRole('button', { name: /Send \(long-press/ })
  const [textBox, attachBox, sendBox] = await Promise.all([
    textarea.boundingBox(),
    attach.boundingBox(),
    send.boundingBox(),
  ])
  expect(Math.abs(textBox!.height - attachBox!.height)).toBeLessThanOrEqual(1)
  expect(Math.abs(textBox!.height - sendBox!.height)).toBeLessThanOrEqual(1)

  await textarea.fill('one\ntwo\nthree\nfour')
  const metrics = await textarea.evaluate((element) => {
    const style = getComputedStyle(element)
    return {
      height: element.getBoundingClientRect().height,
      maxThreeLines:
        3 * Number.parseFloat(style.lineHeight) +
        Number.parseFloat(style.paddingTop) +
        Number.parseFloat(style.paddingBottom),
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
    }
  })
  expect(metrics.height).toBeLessThanOrEqual(metrics.maxThreeLines + 1)
  expect(metrics.scrollHeight).toBeGreaterThan(metrics.clientHeight)
})
