import { expect, test } from '@playwright/test'

for (const { modifier, input, expected } of [
  { modifier: 'Ctrl', input: 'c', expected: [3] },
  { modifier: 'Alt', input: 'c', expected: [27, 99] },
]) {
  test(`${modifier} latches for one soft-key input and second tap cancels it`, async ({ page }) => {
    await page.addInitScript(() => {
      const sent: number[][] = []
      ;(window as any).__inputFrames = sent
      const send = WebSocket.prototype.send
      WebSocket.prototype.send = function (data) {
        const bytes = data instanceof ArrayBuffer
          ? new Uint8Array(data)
          : ArrayBuffer.isView(data)
            ? new Uint8Array(data.buffer, data.byteOffset, data.byteLength)
            : null
        if (bytes?.[0] === 'i'.charCodeAt(0)) sent.push([...bytes.slice(1)])
        return send.call(this, data)
      }
    })

    await page.goto('/')
    await page.getByRole('button', { name: /Input mode:/ }).click()

    const chip = page.getByRole('button', { name: modifier, exact: true })
    await chip.click()
    await expect(chip).toHaveAttribute('aria-pressed', 'true')
    await chip.click()
    await expect(chip).toHaveAttribute('aria-pressed', 'false')

    await chip.click()
    await page.getByRole('textbox', { name: 'Terminal input' }).evaluate((textarea, data) => {
      textarea.dispatchEvent(new InputEvent('input', { data, inputType: 'insertText', bubbles: true }))
    }, input)

    await expect(chip).toHaveAttribute('aria-pressed', 'false')
    await expect.poll(() => page.evaluate(() => (window as any).__inputFrames)).toContainEqual(expected)
  })
}
