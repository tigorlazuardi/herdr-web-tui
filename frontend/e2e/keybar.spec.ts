import { expect, test } from '@playwright/test'

test('Ctrl latches for one soft-key input and second tap cancels it', async ({ page }) => {
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

  const ctrl = page.getByRole('button', { name: 'Ctrl', exact: true })
  await ctrl.click()
  await expect(ctrl).toHaveAttribute('aria-pressed', 'true')
  await ctrl.click()
  await expect(ctrl).toHaveAttribute('aria-pressed', 'false')

  await ctrl.click()
  await page.getByRole('textbox', { name: 'Terminal input' }).evaluate((textarea) => {
    textarea.dispatchEvent(new InputEvent('input', { data: 'c', inputType: 'insertText', bubbles: true }))
  })

  await expect(ctrl).toHaveAttribute('aria-pressed', 'false')
  await expect.poll(() => page.evaluate(() => (window as any).__inputFrames)).toContainEqual([3])
})
