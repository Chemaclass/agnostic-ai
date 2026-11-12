import { expect, test, type Page } from '@playwright/test'

// The list's ⋯ menu and the hidden circle, on the real companion pages served by companion/dev/preview.mjs.

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
    menu.getByRole('switch', { name: 'Animate companion' }),
    menu.getByRole('switch', { name: 'Dim when idle' }),
    menu.getByRole('switch', { name: 'Start at login' }),
    menu.getByRole('button', { name: 'Open from anywhere' }),
    menu.getByRole('button', { name: /^Hide Agnostic AI companion/ }),
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

test('Hide in the menu names the shortcut and asks the shell to hide', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  const hide = page.getByRole('button', { name: 'Hide Agnostic AI companion (⌃⌥Space brings it back)' })
  await expect(hide).toBeVisible()
  const hidden = page.waitForRequest((request) => request.method() === 'POST' && new URL(request.url()).pathname === '/api/hide')
  await hide.click()
  await hidden
  const appearance = await (await page.request.get('/api/appearance')).json()
  expect(appearance.hidden).toBe(true)
})

test('Hide follows a new shortcut', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  await page.request.post('/api/preferences/shortcut', { data: { shortcut: 'Ctrl+Shift+K' } })

  await expect(page.getByRole('button', { name: 'Hide Agnostic AI companion (⌃⇧K brings it back)' })).toBeVisible()
})

test('a hidden circle drops its count, and the shortcut brings it back with a new count', async ({ page }) => {
  await openCircle(page)
  await expect(page.locator('#badge')).toHaveText('2')

  expect((await page.request.post('/api/hide')).status()).toBe(204)

  await expect(page.locator('html')).toHaveAttribute('data-hidden', 'on')
  await expect(page.locator('#badge')).toBeHidden()

  const inboxRead = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/inbox')
  await page.request.post('/api/panel/toggle')
  await inboxRead
  await expect(page.locator('#badge')).toHaveText('2')
  await expect(page.getByRole('button', { name: 'Conversations: 2 need you' })).toBeVisible()
})

test('a circle shown again reads a new list even before any news of one', async ({ page }) => {
  await openCircle(page)
  await expect(page.locator('#badge')).toHaveText('2')
  await page.request.post('/api/hide')
  await expect(page.locator('#badge')).toBeHidden()

  const inboxRead = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/inbox')
  // Only the appearance changes here: no inbox event follows.
  await page.request.post('/dev/appearance', { data: { hidden: false } })
  await inboxRead
  await expect(page.locator('#badge')).toHaveText('2')
})

test('a hidden circle reads nothing when told the list changed', async ({ page }) => {
  await openCircle(page)
  await expect(page.locator('#badge')).toHaveText('2')
  await page.request.post('/api/hide')
  await expect(page.locator('html')).toHaveAttribute('data-hidden', 'on')

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

const settingsMenu = (page: Page) => page.getByRole('region', { name: 'Settings', exact: true })

test('the menu shows the shared list, in its order, with the values in effect', async ({ page }) => {
  const items: { label: string }[] = await (await page.request.get('/menu.json')).json()
  await openPanel(page)
  await openSettings(page)

  const labels = await settingsMenu(page).locator('.menu-label, .menu-item').allTextContents()
  expect(labels).toEqual(items.map((item) => (item.label.startsWith('Hide') ? `${item.label} (⌃⌥Space brings it back)` : item.label)))
  await expect(settingsMenu(page).getByRole('switch', { name: 'Animate companion' })).toHaveAttribute('aria-checked', 'true')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Dim when idle' })).toHaveAttribute('aria-checked', 'false')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Start at login' })).toHaveAttribute('aria-checked', 'false')
})

test('switching Animate in the menu stops the circle on its own page', async ({ page, context }) => {
  await openPanel(page)
  const circlePage = await context.newPage()
  await circlePage.goto('/')
  await expect(circlePage.locator('html')).toHaveAttribute('data-motion', 'on')
  await openSettings(page)

  const saved = page.waitForRequest((request) => new URL(request.url()).pathname === '/api/appearance/animate')
  await settingsMenu(page).getByRole('switch', { name: 'Animate companion' }).click()
  expect((await saved).postDataJSON()).toEqual({ enabled: false })

  await expect(settingsMenu(page).getByRole('switch', { name: 'Animate companion' })).toHaveAttribute('aria-checked', 'false')
  await expect(circlePage.locator('html')).toHaveAttribute('data-motion', 'off')
})

test('switching Dim when idle in the menu saves it', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  await settingsMenu(page).getByRole('switch', { name: 'Dim when idle' }).click()

  await expect(settingsMenu(page).getByRole('switch', { name: 'Dim when idle' })).toHaveAttribute('aria-checked', 'true')
  expect((await (await page.request.get('/api/appearance')).json()).dimWhenIdle).toBe(true)
})

test('a change made in the right-click menu shows in the open menu', async ({ page }) => {
  await openPanel(page)
  await openSettings(page)

  // The tests post what the right-click menu would change.
  await page.request.post('/dev/appearance', { data: { animate: false, dimWhenIdle: true } })
  await page.request.post('/api/preferences/start-at-login', { data: { enabled: true } })

  await expect(settingsMenu(page).getByRole('switch', { name: 'Animate companion' })).toHaveAttribute('aria-checked', 'false')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Dim when idle' })).toHaveAttribute('aria-checked', 'true')
  await expect(settingsMenu(page).getByRole('switch', { name: 'Start at login' })).toHaveAttribute('aria-checked', 'true')
})

test('Open from anywhere in the right-click menu opens the menu on the shortcut, without recording', async ({ page }) => {
  await openPanel(page)

  await page.request.post('/dev/open-settings')

  await expect(settingsMenu(page)).toBeVisible()
  const shortcut = settingsMenu(page).getByRole('button', { name: 'Open from anywhere' })
  await expect(shortcut).toBeFocused()
  await expect(shortcut).not.toHaveClass(/is-recording/)
})
