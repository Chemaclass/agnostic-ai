import { expect, test, type Page } from '@playwright/test'

// The list's ⋯ menu and the paused circle, on the real companion pages served by companion/dev/preview.mjs.

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

test('the keys move through the menu and end on Quit, after Pause', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  await page.getByRole('button', { name: 'Settings', exact: true }).focus()
  const menu = page.getByRole('region', { name: 'Settings', exact: true })
  const order = [
    menu.getByRole('switch', { name: 'Start at login' }),
    menu.getByRole('button', { name: 'Open from anywhere' }),
    menu.getByRole('button', { name: 'Pause Agnostic AI companion' }),
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

async function openCircle(page: Page, fixture = 'mixed') {
  const reset = await page.request.post('/dev/fixture', { data: { name: fixture } })
  expect(reset.ok()).toBe(true)
  await page.goto('/')
}

const circle = (page: Page) => page.locator('#bubble')

test('Pause in the menu asks the shell to pause', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  const paused = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/pause')
  await page.getByRole('button', { name: 'Pause Agnostic AI companion' }).click()
  await paused
  const appearance = await (await page.request.get('/api/appearance')).json()
  expect(appearance.paused).toBe(true)
})

test('a paused circle fades to 10% with no count, and a click wakes it with a new count', async ({ page }) => {
  await openCircle(page)
  await expect(page.locator('#badge')).toHaveText('2')
  await expect(circle(page)).toHaveCSS('opacity', '1')

  expect((await page.request.post('/api/pause')).status()).toBe(204)

  await expect(circle(page)).toHaveCSS('opacity', '0.1')
  await expect(page.locator('#badge')).toBeHidden()
  await expect(page.locator('.ring')).toBeHidden()
  await expect(page.getByRole('button', { name: 'Conversations: paused. Click to resume.' })).toBeVisible()
  await circle(page).hover()
  await expect(circle(page)).toHaveCSS('opacity', '0.1')

  const inboxRead = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/inbox')
  await circle(page).click()
  await inboxRead
  await expect(circle(page)).toHaveCSS('opacity', '1')
  await expect(page.locator('#badge')).toHaveText('2')
  await expect(page.getByRole('button', { name: 'Conversations: 2 need you' })).toBeVisible()
})

test('a paused circle reads nothing when told the list changed', async ({ page }) => {
  await openCircle(page)
  await expect(page.locator('#badge')).toHaveText('2')
  await page.request.post('/api/pause')
  await expect(circle(page)).toHaveCSS('opacity', '0.1')

  let reads = 0
  page.on('request', (request) => {
    if (new URL(request.url()).pathname === '/api/inbox') reads++
  })
  await page.request.post('/dev/arrive')
  // The arrival's news reaches the page over the event stream; give it time to answer.
  await page.waitForTimeout(300)
  expect(reads).toBe(0)
  await expect(page.locator('#badge')).toBeHidden()
})

test('a pause wins over Dim when idle: 10%, not 55%', async ({ page }) => {
  await openCircle(page, 'empty')
  await page.request.post('/dev/appearance', { data: { dimWhenIdle: true } })
  await expect(circle(page)).toHaveCSS('opacity', '0.55')

  await page.request.post('/api/pause')

  await expect(circle(page)).toHaveCSS('opacity', '0.1')
})
