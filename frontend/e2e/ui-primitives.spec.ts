import { expect, test } from '@playwright/test'

test('Toolbar ArrowRight follows compact controls in visual order', async ({ page }) => {
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
  const preview = page.getByRole('button', { name: 'Preview focused pane' })
  const mode = page.getByRole('button', { name: /Input mode:/ })
  const menu = page.getByRole('button', { name: 'Open app menu' })
  await menu.focus()
  await page.keyboard.press('ArrowRight')
  await expect(decrease).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(increase).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(preview).toBeFocused()
  await page.keyboard.press('ArrowRight')
  await expect(mode).toBeFocused()

  await expect(mode).toHaveAttribute('aria-pressed', 'false')
  await page.keyboard.press('Space')
  await expect(mode).toHaveAttribute('aria-pressed', 'true')
  await expect(mode).toContainText('Rail')
})

test('app menu closes on one backdrop tap and exposes PWA status', async ({ page }) => {
  await page.goto('/')
  const menu = page.getByRole('dialog', { name: 'App menu' })
  await page.getByRole('button', { name: 'Open app menu' }).click()
  await expect(page.getByRole('button', { name: /Push (on|off)/ })).toBeVisible()
  await page.locator('.drawer-backdrop').click({ position: { x: 380, y: 400 } })
  await expect(menu).toBeHidden()

  await page.getByRole('button', { name: 'Open app menu' }).click()
  await page.getByRole('button', { name: 'PWA permissions' }).click()
  await expect(menu).toBeHidden()
  await expect(page.getByRole('dialog', { name: 'PWA status' })).toBeVisible()
})

test('sidebar wake lock defaults on and can be toggled for the current session', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Open app menu' }).click()
  const wakeLock = page.getByRole('switch', { name: 'Keep screen awake' })

  await expect(wakeLock).toBeChecked()
  await wakeLock.click()
  await expect(wakeLock).not.toBeChecked()
  await page.getByRole('button', { name: 'PWA permissions' }).click()
  await expect(page.getByText('Disabled by user')).toBeVisible()

  await page.getByRole('button', { name: 'Close PWA status' }).click()
  await page.getByRole('button', { name: 'Open app menu' }).click()
  await wakeLock.click()
  await expect(wakeLock).toBeChecked()
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

test('pane preview makes detected URLs directly navigable', async ({ page }) => {
  await page.route('**/api/pane-preview/*', (route) =>
    route.fulfill({ json: { text: 'Server live: http://192.168.100.5:3002.' } }),
  )
  await page.goto('/')
  await page.getByRole('button', { name: 'Preview focused pane' }).click()

  const link = page.getByRole('link', { name: 'http://192.168.100.5:3002' })
  await expect(link).toHaveAttribute('href', 'http://192.168.100.5:3002')
  await expect(link).toHaveAttribute('target', '_blank')
})
