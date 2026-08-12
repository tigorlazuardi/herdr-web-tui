import { expect, test } from '@playwright/test'

test('Toolbar ArrowRight reaches enabled Push and operable mode Toggle in visual order', async ({ page }) => {
  await page.addInitScript(() => {
    const subscription = { endpoint: 'https://push.example/current' }
    const registration = { pushManager: { getSubscription: async () => subscription } }
    Object.defineProperty(window, 'PushManager', { configurable: true, value: class {} })
    Object.defineProperty(window, 'Notification', { configurable: true, value: { permission: 'granted' } })
    Object.defineProperty(navigator, 'serviceWorker', {
      configurable: true,
      value: { register: async () => registration, addEventListener() {}, removeEventListener() {} },
    })
  })
  await page.route('**/api/push/config', (route) => route.fulfill({ json: { enabled: true } }))
  await page.goto('/')
  await expect(page.getByRole('toolbar', { name: 'Terminal controls' })).toBeVisible()

  const decrease = page.getByRole('button', { name: 'Decrease font size' })
  const increase = page.getByRole('button', { name: 'Increase font size' })
  const push = page.getByRole('button', { name: 'Push on' })
  const preview = page.getByRole('button', { name: 'Preview focused pane' })
  const mode = page.getByRole('button', { name: /Input mode:/ })
  await expect(push).toBeEnabled()
  await decrease.focus()
  await page.keyboard.press('ArrowRight')
  await expect(increase).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(push).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(preview).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(mode).toBeFocused()

  await expect(mode).toHaveAttribute('aria-pressed', 'false')
  await page.keyboard.press('Space')
  await expect(mode).toHaveAttribute('aria-pressed', 'true')
  await expect(mode).toContainText('Rail')
})

test('promptbox stays inside initial app viewport before terminal interaction', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 })
  await page.goto('/')
  await page.addStyleTag({ content: 'html { height: 744px !important; }' })

  const appBottom = await page.locator('html').evaluate((element) => element.getBoundingClientRect().bottom)
  const promptBottom = await page.locator('.promptbox').evaluate((element) => element.getBoundingClientRect().bottom)
  expect(promptBottom).toBeCloseTo(appBottom, 0)
})

test('pane preview uses modal dialog Escape behavior', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Preview focused pane' }).click()
  const dialog = page.getByRole('dialog', { name: 'Focused pane preview' })
  await expect(dialog).toBeVisible()

  await page.keyboard.press('Escape')
  await expect(dialog).toBeHidden()
})
