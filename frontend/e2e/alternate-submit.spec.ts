import { expect, test } from '@playwright/test'

async function mockSend(page: import('@playwright/test').Page, bodies: string[]) {
  await page.route('**/send', async (route) => {
    bodies.push(route.request().postData() ?? '')
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ ok: true }),
    })
  })
}

test('normal Send tap submits Enter once', async ({ page }) => {
  const bodies: string[] = []
  await mockSend(page, bodies)
  await page.goto('/')
  await page.getByRole('textbox', { name: 'Message' }).fill('hello')
  await page.getByRole('button', { name: /^Send/ }).click()

  await expect.poll(() => bodies.length).toBe(1)
  expect(bodies[0]).toContain('name="submitKey"\r\n\r\nenter')
})

test('long-press suppresses normal click and submits selected alternate once', async ({ page }) => {
  const bodies: string[] = []
  await mockSend(page, bodies)
  await page.goto('/')
  await page.getByRole('textbox', { name: 'Message' }).fill('hello')
  const send = page.getByRole('button', { name: /^Send/ })
  const box = await send.boundingBox()
  if (!box) throw new Error('Send button has no box')

  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2)
  await page.mouse.down()
  await page.waitForTimeout(550)
  await page.mouse.up()
  await expect(page.getByText('Submit with')).toBeVisible()
  expect(bodies).toHaveLength(0)

  await page.getByRole('button', { name: 'Ctrl+Enter' }).click()
  await expect.poll(() => bodies.length).toBe(1)
  expect(bodies[0]).toContain('name="submitKey"\r\n\r\nctrl-enter')
})

test('moved cancelled gesture does not swallow next normal Send tap', async ({ page }) => {
  const bodies: string[] = []
  await mockSend(page, bodies)
  await page.goto('/')
  await page.getByRole('textbox', { name: 'Message' }).fill('hello')
  const send = page.getByRole('button', { name: /^Send/ })

  await send.evaluate((button) => {
    button.setPointerCapture = () => {}
    button.dispatchEvent(new PointerEvent('pointerdown', { bubbles: true, button: 0, pointerId: 7, clientX: 10, clientY: 10 }))
    button.dispatchEvent(new PointerEvent('pointermove', { bubbles: true, pointerId: 7, clientX: 30, clientY: 10 }))
    button.dispatchEvent(new PointerEvent('pointercancel', { bubbles: true, pointerId: 7, clientX: 30, clientY: 10 }))
  })

  await expect(page.getByText('Submit with')).toBeHidden()
  expect(bodies).toHaveLength(0)
  await send.click()
  await expect.poll(() => bodies.length).toBe(1)
  expect(bodies[0]).toContain('name="submitKey"\r\n\r\nenter')
})

test('keyboard activation opens alternate submit choices', async ({ page }) => {
  await page.goto('/')
  const send = page.getByRole('button', { name: /^Send/ })
  await send.focus()
  await page.keyboard.press('Enter')

  await expect(page.getByText('Submit with')).toBeVisible()
  await expect(page.getByRole('button', { name: 'Ctrl+Enter' })).toBeEnabled()
  await page.keyboard.press('Escape')
  await expect(page.getByText('Submit with')).toBeHidden()
})
