import { expect, test } from '@playwright/test'

// The thin indeterminate bar above the promptbox is the only feedback that
// a send (upload + server-side remote attachment sync) is in flight; these
// tests pin its lifecycle to the in-flight window.

test('progress bar appears while the send is pending and disappears on success', async ({
  page,
}) => {
  let release!: () => void
  const gate = new Promise<void>((resolve) => {
    release = resolve
  })
  // Hold the /send response open so the in-flight window is observable.
  await page.route('**/send', async (route) => {
    await gate
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ ok: true }),
    })
  })
  await page.goto('/')
  const bar = page.getByRole('progressbar', { name: 'Sending' })
  await expect(bar).toHaveCount(0)

  await page.getByRole('textbox', { name: 'Message' }).fill('hello')
  await page.getByRole('button', { name: /^Send/ }).click()

  await expect(bar).toBeVisible()

  release()
  await expect(bar).toHaveCount(0)
})

test('progress bar disappears on send error', async ({ page }) => {
  await page.route('**/send', (route) =>
    route.fulfill({
      status: 500,
      contentType: 'application/json',
      body: JSON.stringify({ ok: false, error: 'inject failed' }),
    }),
  )
  await page.goto('/')
  await page.getByRole('textbox', { name: 'Message' }).fill('hello')
  await page.getByRole('button', { name: /^Send/ }).click()

  const bar = page.getByRole('progressbar', { name: 'Sending' })
  await expect(bar).toHaveCount(0)
  await expect(page.getByRole('alert')).toBeVisible()
})
