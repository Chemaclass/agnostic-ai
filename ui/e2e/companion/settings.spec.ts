import { expect, test, type Page } from '@playwright/test'

// The list's ⋯ menu, on the real companion pages served by companion/dev/preview.mjs.

async function openPanel(page: Page) {
  const reset = await page.request.post('/dev/fixture', { data: { name: 'mixed' } })
  expect(reset.ok()).toBe(true)
  await page.goto('/panel.html')
  await expect(page.locator('#list > .row, #list > .empty').first()).toBeVisible()
}

async function openSettings(page: Page) {
  await page.getByRole('button', { name: 'Settings', exact: true }).click()
  await expect(page.getByRole('region', { name: 'Settings', exact: true })).toBeVisible()
}

function sent(page: Page, path: string) {
  const paths: string[] = []
  page.on('request', (request) => {
    if (request.method() === 'POST' && new URL(request.url()).pathname === path) paths.push(path)
  })
  return paths
}

test('the keys move through the menu and end on Quit, after Hide', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  await page.getByRole('button', { name: 'Settings', exact: true }).focus()
  const menu = page.getByRole('region', { name: 'Settings', exact: true })
  const order = [
    menu.getByRole('switch', { name: 'Start at login' }),
    menu.getByRole('button', { name: 'Open from anywhere' }),
    menu.getByRole('button', { name: 'Hide Agnostic AI companion' }),
    menu.getByRole('button', { name: 'Quit Agnostic AI companion' }),
  ]
  for (const item of order) {
    await page.keyboard.press('Tab')
    await expect(item).toBeFocused()
  }
})

test('Escape closes the menu and keeps the list; a second Escape hides the list', async ({ page }) => {
  await openPanel(page)
  const hides = sent(page, '/api/panel/hide')
  await openSettings(page)

  await page.keyboard.press('Escape')
  await expect(page.getByRole('region', { name: 'Settings', exact: true })).toBeHidden()
  await expect(page.getByLabel('Search every conversation')).toBeVisible()
  expect(hides).toEqual([])

  const hidden = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/panel/hide')
  await page.keyboard.press('Escape')
  await hidden
})

test('Quit asks the shell to end the companion', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  const quit = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/quit')
  await page.getByRole('button', { name: 'Quit Agnostic AI companion' }).click()
  await quit
  await expect.poll(async () => (await (await page.request.get('/dev/calls')).json())[0]?.path).toBe('/api/quit')
})
